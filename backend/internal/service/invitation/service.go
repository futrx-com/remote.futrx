package invitation

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	serviceemail "github.com/futrx-com/remote.futrx.com/internal/service/email"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
)

// deliveryTimeout bounds the synchronous SMTP send an administrator waits on.
const deliveryTimeout = 20 * time.Second

// Users is the user directory the service adds to, reads and removes from.
type Users interface {
	Add(ctx context.Context, email string, role serviceuser.Role, addedBy string) (serviceuser.User, error)
	Get(ctx context.Context, email string) (*serviceuser.User, error)
	Remove(ctx context.Context, email string) error
}

// Credentials issues and deletes invited accounts' password credentials.
// Issuing also signs the account out everywhere.
type Credentials interface {
	IssueTemporaryPassword(ctx context.Context, email string) (string, error)
	DeleteAccountCredential(ctx context.Context, email string) error
}

// Policy is the sign-in configuration the service has to respect.
type Policy interface {
	IsLocalAdmin(email string) bool
	RequireGoogleSignIn() error
}

// Mailer is what the service needs of the mail facade.
type Mailer interface {
	Enabled(ctx context.Context) bool
	Mail() *serviceemail.Mail
}

// Dependencies are the collaborators of Service. BaseURL is the sign-in URL
// put in the email.
type Dependencies struct {
	Users       Users
	Credentials Credentials
	Policy      Policy
	Mailer      Mailer
	BaseURL     string
}

// Service creates, resets and removes accounts. One lock per email serializes
// all three, so a reset can never leave a credential behind a removal and a
// removal cannot interleave with an invite of the same address.
type Service struct {
	users       Users
	credentials Credentials
	policy      Policy
	mailer      Mailer
	baseURL     string
	// timeout bounds one send; tests shorten it.
	timeout time.Duration

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func New(deps Dependencies) *Service {
	return &Service{
		users:       deps.Users,
		credentials: deps.Credentials,
		policy:      deps.Policy,
		mailer:      deps.Mailer,
		baseURL:     deps.BaseURL,
		timeout:     deliveryTimeout,
		locks:       make(map[string]*sync.Mutex),
	}
}

// lock blocks until the caller owns email's lock and returns its unlock func.
func (s *Service) lock(email string) func() {
	s.mu.Lock()
	mu := s.locks[email]
	if mu == nil {
		mu = &sync.Mutex{}
		s.locks[email] = mu
	}
	s.mu.Unlock()
	mu.Lock()
	return mu.Unlock
}

// Invite adds a user. A Google invite only registers the address (and drops
// any credential left under it, so a re-invite can never inherit an old
// password). A password invite also issues a temporary password and delivers
// it as asked; the account is rolled back if the password cannot be issued.
func (s *Service) Invite(ctx context.Context, req InviteRequest) (Result, error) {
	method := req.Method
	if method == "" {
		method = MethodGoogle
	}
	if method != MethodGoogle && method != MethodPassword {
		return Result{}, ErrInvalidMethod
	}
	email := serviceuser.NormalizeEmail(req.Email)
	defer s.lock(email)()

	if method == MethodGoogle {
		return s.inviteGoogle(ctx, email, req)
	}
	return s.invitePassword(ctx, email, req)
}

func (s *Service) inviteGoogle(ctx context.Context, email string, req InviteRequest) (Result, error) {
	if err := s.policy.RequireGoogleSignIn(); err != nil {
		return Result{}, err
	}
	user, err := s.users.Add(ctx, email, req.Role, req.InvitedBy)
	if err != nil {
		return Result{}, err
	}
	// Add refused every registered address, so a credential found now belongs
	// to an earlier incarnation of the account. If it cannot be deleted the
	// user is rolled back rather than left able to sign in with it.
	if err := s.credentials.DeleteAccountCredential(ctx, email); err != nil {
		return Result{}, s.rollback(ctx, email, fmt.Errorf("clear stale credential: %w", err))
	}
	return Result{User: user, Method: MethodGoogle}, nil
}

func (s *Service) invitePassword(ctx context.Context, email string, req InviteRequest) (Result, error) {
	user, err := s.users.Add(ctx, email, req.Role, req.InvitedBy)
	if err != nil {
		return Result{}, err
	}
	password, err := s.credentials.IssueTemporaryPassword(ctx, email)
	if err != nil {
		return Result{}, s.rollback(ctx, email, fmt.Errorf("issue temporary password: %w", err))
	}
	result := Result{User: user, Method: MethodPassword, TemporaryPassword: password}
	result.Delivery, result.DeliveryError = s.deliver(ctx, email, password, req.SendEmail, false)
	return result, nil
}

// rollback removes a user whose invite failed after Add. Its cause is
// returned inside ErrAccountNotCreated; if the removal fails too, both
// failures are joined and logged, because the half-created user is then
// visible to the administrator and must be removed by hand.
func (s *Service) rollback(ctx context.Context, email string, cause error) error {
	// A cancelled request must not strand the half-created user.
	if err := s.users.Remove(context.WithoutCancel(ctx), email); err != nil {
		log.Printf("invitation: roll back %s failed: %v (after: %v)", maskEmail(email), err, cause)
		return errors.Join(ErrAccountNotRolledBack, cause, fmt.Errorf("remove user: %w", err))
	}
	log.Printf("invitation: rolled back %s: %v", maskEmail(email), cause)
	return errors.Join(ErrAccountNotCreated, cause)
}

// Reset gives an existing user a new temporary password, invalidating the old
// one, any open set-password step and their sessions. A Google-only user gains
// a password credential. The administrator themselves and the local
// administrator are refused.
func (s *Service) Reset(ctx context.Context, req ResetRequest) (Result, error) {
	email := serviceuser.NormalizeEmail(req.Email)
	if email != "" && email == serviceuser.NormalizeEmail(req.Actor) {
		return Result{}, ErrResetSelf
	}
	if s.policy.IsLocalAdmin(email) {
		return Result{}, ErrResetLocalAdmin
	}
	defer s.lock(email)()

	// Looked up under the lock: a removal that won the race makes this a 404
	// instead of issuing a credential for a user who is gone.
	user, err := s.users.Get(ctx, email)
	if err != nil {
		return Result{}, err
	}
	if user == nil {
		return Result{}, serviceuser.ErrUserNotFound
	}
	password, err := s.credentials.IssueTemporaryPassword(ctx, email)
	if err != nil {
		return Result{}, fmt.Errorf("issue temporary password: %w", err)
	}
	result := Result{User: *user, Method: MethodPassword, TemporaryPassword: password}
	result.Delivery, result.DeliveryError = s.deliver(ctx, email, password, req.SendEmail, true)
	return result, nil
}

// Remove deletes a user and everything keyed by them, including their
// credential, under the same per-email lock as Invite and Reset.
func (s *Service) Remove(ctx context.Context, email string) error {
	email = serviceuser.NormalizeEmail(email)
	defer s.lock(email)()
	return s.users.Remove(ctx, email)
}

// deliver emails the credentials when asked and possible, and says what
// happened. It never returns the transport's own error text.
func (s *Service) deliver(ctx context.Context, email, password string, requested, reset bool) (Delivery, string) {
	if !requested {
		return DeliveryNotRequested, ""
	}
	if !s.mailer.Enabled(ctx) {
		return DeliveryNotConfigured, ""
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	err := s.compose(email, password, reset).Send(ctx)
	switch {
	case err == nil:
		return DeliverySent, ""
	case errors.Is(err, serviceemail.ErrNotConfigured):
		// Configuration removed between Enabled and Send.
		return DeliveryNotConfigured, ""
	}
	log.Printf("invitation: email to %s not delivered: %s", maskEmail(email), classifyDeliveryError(err))
	return DeliveryFailed, classifyDeliveryError(err)
}

func (s *Service) compose(email, password string, reset bool) *serviceemail.Mail {
	subject, intro := "You've been invited to Remote",
		"An administrator created a Remote account for you. Sign in with your email address and the temporary password below."
	if reset {
		subject, intro = "Your Remote password was reset",
			"An administrator reset your Remote password. Sign in with your email address and the temporary password below."
	}
	return s.mailer.Mail().
		To(email).
		Required().
		Subject(subject).
		Heading(subject).
		Text(intro).
		KeyValues([2]string{"Sign-in URL", s.baseURL}, [2]string{"Email", email}).
		Code(password).
		Button("Sign in", s.baseURL).
		Note("You'll be asked to choose a new password the first time you sign in. If you didn't expect this, ignore this email.")
}

// classifyDeliveryError maps a transport failure to one of three fixed
// descriptions. The transport wraps its cause as text, so the stage is read
// from the message; the message itself is never passed on.
func classifyDeliveryError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return deliveryErrorUnreachable
	}
	text := err.Error()
	for _, stage := range []string{"smtp: dial", "smtp: set deadline", "smtp: new client", "smtp: hello", "smtp: starttls", "i/o timeout", "deadline exceeded"} {
		if strings.Contains(text, stage) {
			return deliveryErrorUnreachable
		}
	}
	if errors.Is(err, serviceemail.ErrSendFailed) {
		for _, stage := range []string{"smtp: mail from", "smtp: rcpt to", "smtp: data", "smtp: write body", "smtp: close data"} {
			if strings.Contains(text, stage) {
				return deliveryErrorRejected
			}
		}
	}
	return deliveryErrorGeneric
}

// maskEmail keeps only the first character of the local part, so a log line
// shows that an address was involved without recording it:
// "j***@example.com".
func maskEmail(email string) string {
	at := strings.IndexByte(email, '@')
	if at <= 0 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}
