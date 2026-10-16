package invitation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	emailapplication "github.com/futrx-com/remote.futrx.com/internal/model/email/application"
	emaildomain "github.com/futrx-com/remote.futrx.com/internal/model/email/domain"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceemail "github.com/futrx-com/remote.futrx.com/internal/service/email"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
)

const (
	testEmail    = "invitee@example.com"
	testPassword = "TEMP7-PASSW-ORD22-ABCDE"
	testBaseURL  = "https://remote.example.com"
	adminEmail   = "boss@example.com"
	localAdmin   = "local@example.com"
)

// fakeUsers is the user directory. Remove runs onRemove, which stands in for
// the removal cleanup that deletes the account credential.
type fakeUsers struct {
	mu        sync.Mutex
	users     map[string]serviceuser.User
	removeErr error
	onRemove  func(email string)
	removed   []string
	probe     *probe
}

func (u *fakeUsers) Add(_ context.Context, email string, role serviceuser.Role, addedBy string) (serviceuser.User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	email = serviceuser.NormalizeEmail(email)
	if !strings.Contains(email, "@") {
		return serviceuser.User{}, serviceuser.ErrInvalidEmail
	}
	if !serviceuser.ValidRole(role) {
		return serviceuser.User{}, serviceuser.ErrInvalidRole
	}
	if _, ok := u.users[email]; ok {
		return serviceuser.User{}, serviceuser.ErrUserExists
	}
	user := serviceuser.User{Email: email, Role: role, AddedBy: serviceuser.NormalizeEmail(addedBy)}
	u.users[email] = user
	return user, nil
}

func (u *fakeUsers) Get(_ context.Context, email string) (*serviceuser.User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	email = serviceuser.NormalizeEmail(email)
	if !strings.Contains(email, "@") {
		return nil, serviceuser.ErrInvalidEmail
	}
	user, ok := u.users[email]
	if !ok {
		return nil, nil
	}
	return &user, nil
}

func (u *fakeUsers) Remove(_ context.Context, email string) error {
	done := u.probe.enter()
	defer done()
	u.mu.Lock()
	defer u.mu.Unlock()
	email = serviceuser.NormalizeEmail(email)
	u.removed = append(u.removed, email)
	if u.removeErr != nil {
		return u.removeErr
	}
	if _, ok := u.users[email]; !ok {
		return serviceuser.ErrUserNotFound
	}
	if u.onRemove != nil {
		u.onRemove(email)
	}
	delete(u.users, email)
	return nil
}

func (u *fakeUsers) has(email string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	_, ok := u.users[email]
	return ok
}

// fakeCredentials holds the issued passwords by email.
type fakeCredentials struct {
	mu         sync.Mutex
	passwords  map[string]string
	issued     int
	issueErr   error
	deleteErr  error
	deleted    []string
	beforeSave func(email string)
	probe      *probe
}

func (c *fakeCredentials) IssueTemporaryPassword(_ context.Context, email string) (string, error) {
	done := c.probe.enter()
	defer done()
	if c.beforeSave != nil {
		c.beforeSave(email)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.issueErr != nil {
		return "", c.issueErr
	}
	c.issued++
	password := testPassword
	if c.issued > 1 {
		password = fmt.Sprintf("TEMP%d-PASSW-ORD22-ABCDE", c.issued)
	}
	c.passwords[email] = password
	return password, nil
}

func (c *fakeCredentials) DeleteAccountCredential(_ context.Context, email string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleted = append(c.deleted, email)
	if c.deleteErr != nil {
		return c.deleteErr
	}
	delete(c.passwords, email)
	return nil
}

func (c *fakeCredentials) has(email string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.passwords[email]
	return ok
}

// probe records the highest number of operations running at once on a
// fixture, which the per-email lock must keep at one for a single address.
type probe struct {
	active atomic.Int64
	max    atomic.Int64
	pause  time.Duration
}

func (p *probe) enter() func() {
	if p == nil {
		return func() {}
	}
	now := p.active.Add(1)
	for {
		seen := p.max.Load()
		if now <= seen || p.max.CompareAndSwap(seen, now) {
			break
		}
	}
	time.Sleep(p.pause)
	return func() { p.active.Add(-1) }
}

type fakePolicy struct {
	googleErr error
}

func (p fakePolicy) IsLocalAdmin(email string) bool { return email == localAdmin }
func (p fakePolicy) RequireGoogleSignIn() error     { return p.googleErr }

type fakeConfigStore struct {
	cfg *emailapplication.SMTPConfiguration
}

func (s *fakeConfigStore) Configuration(context.Context) (*emailapplication.SMTPConfiguration, error) {
	return s.cfg, nil
}
func (s *fakeConfigStore) Save(context.Context, emailapplication.SMTPConfiguration) error { return nil }
func (s *fakeConfigStore) Delete(context.Context) error                                   { return nil }

type fakeSender struct {
	mu       sync.Mutex
	sendErr  error
	messages []emaildomain.Message
	block    func(ctx context.Context) error
}

func (s *fakeSender) Verify(context.Context, emailapplication.SMTPConfiguration) error { return nil }
func (s *fakeSender) Send(ctx context.Context, _ emailapplication.SMTPConfiguration, msg emaildomain.Message) error {
	if s.block != nil {
		return s.block(ctx)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendErr != nil {
		return s.sendErr
	}
	s.messages = append(s.messages, msg)
	return nil
}

func (s *fakeSender) sent() []emaildomain.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]emaildomain.Message(nil), s.messages...)
}

// alwaysEnabled reports SMTP as configured whatever the store holds, to reach
// the window between the check and the send.
type alwaysEnabled struct{ *serviceemail.Mailer }

func (alwaysEnabled) Enabled(context.Context) bool { return true }

// captureHandler collects every record logged through slog and the log
// package, which slog.SetDefault redirects here.
type captureHandler struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&h.buf, " %s=%v", a.Key, a.Value)
		return true
	})
	h.buf.WriteByte('\n')
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }
func (h *captureHandler) String() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.buf.String()
}

type fixture struct {
	svc    *Service
	users  *fakeUsers
	creds  *fakeCredentials
	policy *fakePolicy
	sender *fakeSender
	store  *fakeConfigStore
	logs   *captureHandler
	probe  *probe
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		probe:  &probe{},
		policy: &fakePolicy{},
		sender: &fakeSender{},
		logs:   &captureHandler{},
		store: &fakeConfigStore{cfg: &emailapplication.SMTPConfiguration{
			Host: "smtp.example.com", Port: 587, TLSMode: emailapplication.TLSModeSTARTTLS,
			Authentication: emailapplication.AuthenticationPlain, Username: "server@example.com",
			Password: "smtp-secret-password", FromAddress: "server@example.com",
		}},
	}
	f.users = &fakeUsers{users: map[string]serviceuser.User{}, probe: f.probe}
	f.creds = &fakeCredentials{passwords: map[string]string{}, probe: f.probe}
	f.users.onRemove = func(email string) {
		// What the removal cleanup does.
		delete(f.creds.passwords, email)
	}
	f.svc = New(Dependencies{
		Users: f.users, Credentials: f.creds, Policy: f.policy,
		Mailer:  serviceemail.NewMailer(serviceemail.New(f.store, f.sender), nil),
		BaseURL: testBaseURL,
	})
	old := slog.Default()
	slog.SetDefault(slog.New(f.logs))
	t.Cleanup(func() { slog.SetDefault(old) })
	return f
}

// assertClean fails if any password, or the unmasked address, reached a log or
// one of the errors.
func (f *fixture) assertClean(t *testing.T, errs ...error) {
	t.Helper()
	secrets := []string{testPassword, "TEMP2-PASSW-ORD22-ABCDE", "TEMP3-PASSW-ORD22-ABCDE", "smtp-secret-password"}
	for _, secret := range secrets {
		if strings.Contains(f.logs.String(), secret) {
			t.Fatalf("secret %q leaked to the log:\n%s", secret, f.logs.String())
		}
		for _, err := range errs {
			if err != nil && strings.Contains(err.Error(), secret) {
				t.Fatalf("secret %q leaked in error: %v", secret, err)
			}
		}
	}
	if strings.Contains(f.logs.String(), testEmail) {
		t.Fatalf("unmasked address in the log:\n%s", f.logs.String())
	}
}

func passwordInvite(sendEmail bool) InviteRequest {
	return InviteRequest{Email: testEmail, Role: serviceuser.RoleMember, Method: MethodPassword, SendEmail: sendEmail, InvitedBy: adminEmail}
}

func TestPasswordInviteDeliveryStates(t *testing.T) {
	ctx := context.Background()

	t.Run("sent", func(t *testing.T) {
		f := newFixture(t)
		result, err := f.svc.Invite(ctx, passwordInvite(true))
		if err != nil {
			t.Fatal(err)
		}
		if result.Delivery != DeliverySent || result.DeliveryError != "" || result.TemporaryPassword != testPassword ||
			result.Method != MethodPassword || result.User.Email != testEmail || result.User.Role != serviceuser.RoleMember ||
			result.User.AddedBy != adminEmail {
			t.Fatalf("result = %+v", result)
		}
		if got := f.sender.sent(); len(got) != 1 || got[0].To != testEmail {
			t.Fatalf("sent = %+v", got)
		}
		f.assertClean(t)
	})

	t.Run("not requested", func(t *testing.T) {
		f := newFixture(t)
		result, err := f.svc.Invite(ctx, passwordInvite(false))
		if err != nil || result.Delivery != DeliveryNotRequested || result.TemporaryPassword != testPassword {
			t.Fatalf("result = %+v, %v", result, err)
		}
		if len(f.sender.sent()) != 0 {
			t.Fatal("a mail was sent although none was requested")
		}
	})

	t.Run("not configured", func(t *testing.T) {
		f := newFixture(t)
		f.store.cfg = nil
		result, err := f.svc.Invite(ctx, passwordInvite(true))
		if err != nil || result.Delivery != DeliveryNotConfigured || result.DeliveryError != "" || result.TemporaryPassword != testPassword {
			t.Fatalf("result = %+v, %v", result, err)
		}
		if !f.users.has(testEmail) || !f.creds.has(testEmail) {
			t.Fatal("account missing after an unconfigured send")
		}
	})

	t.Run("configuration removed between the check and the send", func(t *testing.T) {
		f := newFixture(t)
		f.store.cfg = nil
		f.svc.mailer = alwaysEnabled{serviceemail.NewMailer(serviceemail.New(f.store, f.sender), nil)}
		result, err := f.svc.Invite(ctx, passwordInvite(true))
		if err != nil || result.Delivery != DeliveryNotConfigured {
			t.Fatalf("result = %+v, %v", result, err)
		}
	})

	t.Run("failed keeps the account and shows the password", func(t *testing.T) {
		f := newFixture(t)
		f.sender.sendErr = errors.New("smtp: rcpt to: 550 no such user " + testPassword)
		result, err := f.svc.Invite(ctx, passwordInvite(true))
		if err != nil {
			t.Fatalf("a failed delivery must not fail the invite: %v", err)
		}
		if result.Delivery != DeliveryFailed || result.DeliveryError != deliveryErrorRejected || result.TemporaryPassword != testPassword {
			t.Fatalf("result = %+v", result)
		}
		if !f.users.has(testEmail) || !f.creds.has(testEmail) {
			t.Fatal("account rolled back because the mail failed")
		}
		if strings.Contains(result.DeliveryError, "550") || strings.Contains(result.DeliveryError, testPassword) {
			t.Fatalf("raw error text in DeliveryError: %q", result.DeliveryError)
		}
		f.assertClean(t)
	})
}

func TestDeliveryErrorsAreFixedAndNeverRawText(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"recipient rejected": {errors.New("smtp: rcpt to: 550 mailbox unavailable 10.1.2.3"), deliveryErrorRejected},
		"sender rejected":    {errors.New("smtp: mail from: 553 not allowed"), deliveryErrorRejected},
		"data rejected":      {errors.New("smtp: data: 554 spam"), deliveryErrorRejected},
		"dial refused":       {errors.New("smtp: dial: dial tcp 10.1.2.3:587: connection refused"), deliveryErrorUnreachable},
		"tls handshake":      {errors.New("smtp: starttls: tls: handshake failure"), deliveryErrorUnreachable},
		"timeout":            {errors.New("read tcp 10.1.2.3:587: i/o timeout"), deliveryErrorUnreachable},
		"context deadline":   {context.DeadlineExceeded, deliveryErrorUnreachable},
		"auth failure":       {errors.New("smtp: auth: 535 bad credentials for server@example.com"), deliveryErrorGeneric},
		"anything else":      {errors.New("boom /etc/secret.conf"), deliveryErrorGeneric},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.sender.sendErr = tc.err
			result, err := f.svc.Invite(ctx, passwordInvite(true))
			if err != nil || result.Delivery != DeliveryFailed || result.DeliveryError != tc.want {
				t.Fatalf("result = %+v, %v; want %q", result, err, tc.want)
			}
			for _, raw := range []string{"10.1.2.3", "550", "/etc/", "server@example.com", "535"} {
				if strings.Contains(result.DeliveryError, raw) || strings.Contains(f.logs.String(), raw) {
					t.Fatalf("raw transport text %q leaked: %q / %s", raw, result.DeliveryError, f.logs.String())
				}
			}
			f.assertClean(t, err)
		})
	}
}

func TestSlowDeliveryTimesOutAsFailed(t *testing.T) {
	f := newFixture(t)
	f.svc.timeout = 30 * time.Millisecond
	f.sender.block = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	started := time.Now()
	result, err := f.svc.Invite(context.Background(), passwordInvite(true))
	if err != nil || result.Delivery != DeliveryFailed || result.DeliveryError != deliveryErrorUnreachable {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("the send was not bounded")
	}
	if !f.users.has(testEmail) || result.TemporaryPassword == "" {
		t.Fatal("a timeout lost the account or its password")
	}
}

func TestDeliveryTimeoutIsTwentySeconds(t *testing.T) {
	if New(Dependencies{}).timeout != 20*time.Second {
		t.Fatal("delivery timeout is not 20 seconds")
	}
}

func TestInviteMailContent(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		reset   bool
		subject string
	}{
		"invite": {false, "You've been invited to Remote"},
		"reset":  {true, "Your Remote password was reset"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			var err error
			if tc.reset {
				f.users.users[testEmail] = serviceuser.User{Email: testEmail, Role: serviceuser.RoleMember}
				_, err = f.svc.Reset(ctx, ResetRequest{Email: testEmail, Actor: adminEmail, SendEmail: true})
			} else {
				_, err = f.svc.Invite(ctx, passwordInvite(true))
			}
			if err != nil {
				t.Fatal(err)
			}
			sent := f.sender.sent()
			if len(sent) != 1 {
				t.Fatalf("sent %d messages", len(sent))
			}
			msg := sent[0]
			if msg.Subject != tc.subject {
				t.Fatalf("subject = %q", msg.Subject)
			}
			for _, body := range []string{msg.Body, html.UnescapeString(msg.HTMLBody)} {
				for _, want := range []string{testBaseURL, testEmail, testPassword, tc.subject, "choose a new password the first time you sign in", "ignore this email"} {
					if !strings.Contains(body, want) {
						t.Fatalf("body lacks %q:\n%s", want, body)
					}
				}
				for _, banned := range []string{"expire", "valid for", "hours", "minutes", "days"} {
					if strings.Contains(strings.ToLower(body), banned) {
						t.Fatalf("body mentions %q although passwords do not expire:\n%s", banned, body)
					}
				}
			}
			f.assertClean(t)
		})
	}
}

func TestDuplicateInviteWritesNoCredentialAndSendsNothing(t *testing.T) {
	f := newFixture(t)
	existing := serviceuser.User{Email: testEmail, Role: serviceuser.RoleAdmin, AddedBy: "someone@example.com"}
	f.users.users[testEmail] = existing
	f.creds.passwords[testEmail] = "existing-hash"

	for _, method := range []Method{MethodPassword, MethodGoogle, ""} {
		req := passwordInvite(true)
		req.Method = method
		req.Email = " Invitee@Example.com "
		_, err := f.svc.Invite(context.Background(), req)
		if !errors.Is(err, serviceuser.ErrUserExists) {
			t.Fatalf("%q duplicate = %v", method, err)
		}
	}
	if f.creds.issued != 0 || len(f.creds.deleted) != 0 {
		t.Fatalf("a duplicate touched credentials: issued %d, deleted %v", f.creds.issued, f.creds.deleted)
	}
	if f.creds.passwords[testEmail] != "existing-hash" {
		t.Fatal("the existing user's credential changed")
	}
	if f.users.users[testEmail] != existing {
		t.Fatal("the existing user changed")
	}
	if len(f.users.removed) != 0 || len(f.sender.sent()) != 0 {
		t.Fatal("a duplicate removed a user or sent mail")
	}
}

func TestInviteValidationErrorsCreateNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for name, tc := range map[string]struct {
		req  InviteRequest
		want error
	}{
		"bad email":  {InviteRequest{Email: "nope", Role: serviceuser.RoleMember, Method: MethodPassword}, serviceuser.ErrInvalidEmail},
		"bad role":   {InviteRequest{Email: testEmail, Role: "root", Method: MethodPassword}, serviceuser.ErrInvalidRole},
		"bad method": {InviteRequest{Email: testEmail, Role: serviceuser.RoleMember, Method: "sms"}, ErrInvalidMethod},
	} {
		if _, err := f.svc.Invite(ctx, tc.req); !errors.Is(err, tc.want) {
			t.Fatalf("%s = %v, want %v", name, err, tc.want)
		}
	}
	if len(f.users.users) != 0 || f.creds.issued != 0 || len(f.users.removed) != 0 {
		t.Fatal("a rejected invite left state behind")
	}
}

func TestCredentialFailureRollsTheUserBack(t *testing.T) {
	f := newFixture(t)
	f.creds.issueErr = errors.New("registry unavailable")

	result, err := f.svc.Invite(context.Background(), passwordInvite(true))
	if !errors.Is(err, ErrAccountNotCreated) || !errors.Is(err, f.creds.issueErr) || errors.Is(err, ErrAccountNotRolledBack) {
		t.Fatalf("err = %v", err)
	}
	if result != (Result{}) {
		t.Fatalf("result = %+v", result)
	}
	if f.users.has(testEmail) || f.creds.has(testEmail) {
		t.Fatal("user or credential left behind")
	}
	if len(f.sender.sent()) != 0 {
		t.Fatal("mail sent for an account that was not created")
	}
	f.assertClean(t, err)

	// The address is free again.
	f.creds.issueErr = nil
	if _, err := f.svc.Invite(context.Background(), passwordInvite(false)); err != nil {
		t.Fatalf("invite after a rollback = %v", err)
	}
}

func TestRollbackSurvivesACancelledRequest(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	f.creds.beforeSave = func(string) { cancel() }
	f.creds.issueErr = context.Canceled
	f.users.users = map[string]serviceuser.User{}

	_, err := f.svc.Invite(ctx, passwordInvite(true))
	if !errors.Is(err, ErrAccountNotCreated) || f.users.has(testEmail) {
		t.Fatalf("err = %v, user present = %v", err, f.users.has(testEmail))
	}
}

func TestDoubleFailureIsReportedAndLoggedMasked(t *testing.T) {
	f := newFixture(t)
	f.creds.issueErr = errors.New("registry unavailable")
	f.users.removeErr = errors.New("users file locked")

	_, err := f.svc.Invite(context.Background(), passwordInvite(true))
	if !errors.Is(err, ErrAccountNotRolledBack) || !errors.Is(err, f.creds.issueErr) || !errors.Is(err, f.users.removeErr) {
		t.Fatalf("err = %v", err)
	}
	if errors.Is(err, ErrAccountNotCreated) {
		t.Fatal("a half-created account was reported as not created")
	}
	for _, want := range []string{"registry unavailable", "users file locked"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error lacks %q: %v", want, err)
		}
	}
	if !f.users.has(testEmail) {
		t.Fatal("setup: the user should remain for the administrator to remove")
	}
	if !strings.Contains(f.logs.String(), "i***@example.com") {
		t.Fatalf("masked address missing from the log:\n%s", f.logs.String())
	}
	f.assertClean(t, err)
}

func TestGoogleInvite(t *testing.T) {
	ctx := context.Background()
	req := InviteRequest{Email: testEmail, Role: serviceuser.RoleMember, Method: MethodGoogle, SendEmail: true, InvitedBy: adminEmail}

	t.Run("requires Google sign-in and creates nothing without it", func(t *testing.T) {
		f := newFixture(t)
		f.policy.googleErr = serviceauth.ErrGoogleSignInRequired
		for _, method := range []Method{MethodGoogle, ""} {
			req := req
			req.Method = method
			if _, err := f.svc.Invite(ctx, req); !errors.Is(err, serviceauth.ErrGoogleSignInRequired) {
				t.Fatalf("%q = %v", method, err)
			}
		}
		if len(f.users.users) != 0 || len(f.creds.deleted) != 0 {
			t.Fatal("state changed although Google is not configured")
		}
		// A password invite does not need Google.
		if _, err := f.svc.Invite(ctx, passwordInvite(false)); err != nil {
			t.Fatalf("password invite without Google = %v", err)
		}
	})

	t.Run("adds the user without a credential or mail", func(t *testing.T) {
		f := newFixture(t)
		result, err := f.svc.Invite(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if result.Method != MethodGoogle || result.TemporaryPassword != "" || result.Delivery != "" || result.User.Email != testEmail {
			t.Fatalf("result = %+v", result)
		}
		if f.creds.issued != 0 || len(f.sender.sent()) != 0 {
			t.Fatal("a Google invite issued a password or sent mail")
		}
	})

	t.Run("deletes a credential left behind", func(t *testing.T) {
		f := newFixture(t)
		f.creds.passwords[testEmail] = "stale-hash"
		if _, err := f.svc.Invite(ctx, req); err != nil {
			t.Fatal(err)
		}
		if f.creds.has(testEmail) {
			t.Fatal("the Google invite inherited an old temporary password")
		}
	})

	t.Run("rolls the user back if the stale credential cannot be deleted", func(t *testing.T) {
		f := newFixture(t)
		f.creds.passwords[testEmail] = "stale-hash"
		f.creds.deleteErr = errors.New("disk full")
		_, err := f.svc.Invite(ctx, req)
		if !errors.Is(err, ErrAccountNotCreated) || !errors.Is(err, f.creds.deleteErr) {
			t.Fatalf("err = %v", err)
		}
		if f.users.has(testEmail) {
			t.Fatal("a user able to sign in with the stale password was left registered")
		}
	})

	t.Run("a failed rollback is reported with both causes", func(t *testing.T) {
		f := newFixture(t)
		f.creds.deleteErr = errors.New("disk full")
		f.users.removeErr = errors.New("users file locked")
		_, err := f.svc.Invite(ctx, req)
		if !errors.Is(err, ErrAccountNotRolledBack) || !errors.Is(err, f.creds.deleteErr) || !errors.Is(err, f.users.removeErr) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestResetGuards(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.users.users[testEmail] = serviceuser.User{Email: testEmail, Role: serviceuser.RoleMember}
	f.users.users[adminEmail] = serviceuser.User{Email: adminEmail, Role: serviceuser.RoleAdmin}
	f.users.users[localAdmin] = serviceuser.User{Email: localAdmin, Role: serviceuser.RoleAdmin}

	for name, tc := range map[string]struct {
		req  ResetRequest
		want error
	}{
		"unknown user":        {ResetRequest{Email: "ghost@example.com", Actor: adminEmail, SendEmail: true}, serviceuser.ErrUserNotFound},
		"invalid email":       {ResetRequest{Email: "nope", Actor: adminEmail}, serviceuser.ErrInvalidEmail},
		"self":                {ResetRequest{Email: adminEmail, Actor: adminEmail}, ErrResetSelf},
		"self, other case":    {ResetRequest{Email: " Boss@Example.com ", Actor: "BOSS@example.com"}, ErrResetSelf},
		"local administrator": {ResetRequest{Email: localAdmin, Actor: adminEmail}, ErrResetLocalAdmin},
		"local, own account":  {ResetRequest{Email: localAdmin, Actor: localAdmin}, ErrResetSelf},
	} {
		if _, err := f.svc.Reset(ctx, tc.req); !errors.Is(err, tc.want) {
			t.Fatalf("%s = %v, want %v", name, err, tc.want)
		}
	}
	if f.creds.issued != 0 || len(f.sender.sent()) != 0 {
		t.Fatal("a refused reset issued a password or sent mail")
	}
	if len(f.users.users) != 3 {
		t.Fatalf("a refused reset changed the users: %+v", f.users.users)
	}
}

func TestResetGivesAGoogleOnlyUserACredential(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.users.users[testEmail] = serviceuser.User{Email: testEmail, Role: serviceuser.RoleMember, AddedBy: adminEmail}
	if f.creds.has(testEmail) {
		t.Fatal("setup: user already has a credential")
	}

	result, err := f.svc.Reset(ctx, ResetRequest{Email: " Invitee@Example.com", Actor: adminEmail, SendEmail: true})
	if err != nil {
		t.Fatal(err)
	}
	if !f.creds.has(testEmail) || result.TemporaryPassword != testPassword || result.Method != MethodPassword ||
		result.Delivery != DeliverySent || result.User.Email != testEmail || result.User.AddedBy != adminEmail {
		t.Fatalf("result = %+v", result)
	}

	// A second reset replaces it.
	again, err := f.svc.Reset(ctx, ResetRequest{Email: testEmail, Actor: adminEmail, SendEmail: false})
	if err != nil || again.TemporaryPassword == result.TemporaryPassword || again.Delivery != DeliveryNotRequested {
		t.Fatalf("second reset = %+v, %v", again, err)
	}
	f.assertClean(t, err)
}

func TestResetDeliveryStatesAndFailure(t *testing.T) {
	ctx := context.Background()
	seed := func(f *fixture) {
		f.users.users[testEmail] = serviceuser.User{Email: testEmail, Role: serviceuser.RoleMember}
	}

	t.Run("not configured", func(t *testing.T) {
		f := newFixture(t)
		seed(f)
		f.store.cfg = nil
		result, err := f.svc.Reset(ctx, ResetRequest{Email: testEmail, Actor: adminEmail, SendEmail: true})
		if err != nil || result.Delivery != DeliveryNotConfigured || result.TemporaryPassword == "" {
			t.Fatalf("result = %+v, %v", result, err)
		}
	})

	t.Run("failed", func(t *testing.T) {
		f := newFixture(t)
		seed(f)
		f.sender.sendErr = errors.New("smtp: dial: refused")
		result, err := f.svc.Reset(ctx, ResetRequest{Email: testEmail, Actor: adminEmail, SendEmail: true})
		if err != nil || result.Delivery != DeliveryFailed || result.DeliveryError != deliveryErrorUnreachable {
			t.Fatalf("result = %+v, %v", result, err)
		}
		f.assertClean(t, err)
	})

	t.Run("credential failure keeps the user and sends nothing", func(t *testing.T) {
		f := newFixture(t)
		seed(f)
		f.creds.issueErr = errors.New("registry unavailable")
		result, err := f.svc.Reset(ctx, ResetRequest{Email: testEmail, Actor: adminEmail, SendEmail: true})
		if err == nil || !errors.Is(err, f.creds.issueErr) || result != (Result{}) {
			t.Fatalf("result = %+v, %v", result, err)
		}
		if !f.users.has(testEmail) || len(f.users.removed) != 0 {
			t.Fatal("a failed reset removed an existing user")
		}
		if len(f.sender.sent()) != 0 {
			t.Fatal("mail sent although no password was issued")
		}
		f.assertClean(t, err)
	})
}

func TestRemoveDeletesTheUserAndTheirCredential(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.Invite(ctx, passwordInvite(false)); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Remove(ctx, " Invitee@Example.com "); err != nil {
		t.Fatal(err)
	}
	if f.users.has(testEmail) || f.creds.has(testEmail) {
		t.Fatal("user or credential survived the removal")
	}
	if err := f.svc.Remove(ctx, testEmail); !errors.Is(err, serviceuser.ErrUserNotFound) {
		t.Fatalf("second removal = %v", err)
	}
	f.users.users["x@example.com"] = serviceuser.User{Email: "x@example.com"}
	f.users.removeErr = serviceuser.ErrCannotRemoveLastAdmin
	if err := f.svc.Remove(ctx, "x@example.com"); !errors.Is(err, serviceuser.ErrCannotRemoveLastAdmin) {
		t.Fatalf("guard error = %v", err)
	}
}

// Invite, Reset and Remove of one address never overlap, whichever way they
// are interleaved; different addresses do not wait for each other.
func TestPerEmailLockSerializesInviteResetAndRemove(t *testing.T) {
	f := newFixture(t)
	f.probe.pause = time.Millisecond
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_, _ = f.svc.Invite(ctx, passwordInvite(false))
		}()
		go func() {
			defer wg.Done()
			_, _ = f.svc.Reset(ctx, ResetRequest{Email: testEmail, Actor: adminEmail})
		}()
		go func() {
			defer wg.Done()
			_ = f.svc.Remove(ctx, testEmail)
		}()
	}
	wg.Wait()

	if got := f.probe.max.Load(); got != 1 {
		t.Fatalf("%d operations on one address ran at once", got)
	}
	// Whatever order won, a credential never outlives its user.
	if f.creds.has(testEmail) && !f.users.has(testEmail) {
		t.Fatal("credential outlived its user")
	}
}

func TestDifferentAddressesDoNotBlockEachOther(t *testing.T) {
	f := newFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	f.creds.beforeSave = func(email string) {
		if email == testEmail {
			close(entered)
			<-release
		}
	}
	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		_, err := f.svc.Invite(ctx, passwordInvite(false))
		done <- err
	}()
	<-entered

	other := make(chan error, 1)
	go func() {
		_, err := f.svc.Invite(ctx, InviteRequest{Email: "other@example.com", Role: serviceuser.RoleMember, Method: MethodPassword})
		other <- err
	}()
	select {
	case err := <-other:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("an invite for another address waited for the first")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// A removal that arrives while a reset is issuing must wait, and then delete
// the credential the reset wrote, so no credential is left for the removed
// address.
func TestResetRacingRemovalLeavesNoCredential(t *testing.T) {
	ctx := context.Background()

	t.Run("removal arrives mid-reset", func(t *testing.T) {
		f := newFixture(t)
		f.users.users[testEmail] = serviceuser.User{Email: testEmail, Role: serviceuser.RoleMember}
		entered := make(chan struct{})
		release := make(chan struct{})
		f.creds.beforeSave = func(string) {
			close(entered)
			<-release
		}
		resetDone := make(chan error, 1)
		go func() {
			_, err := f.svc.Reset(ctx, ResetRequest{Email: testEmail, Actor: adminEmail})
			resetDone <- err
		}()
		<-entered

		removeDone := make(chan error, 1)
		go func() { removeDone <- f.svc.Remove(ctx, testEmail) }()
		select {
		case <-removeDone:
			t.Fatal("removal ran while the reset was still issuing")
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		if err := <-resetDone; err != nil {
			t.Fatal(err)
		}
		if err := <-removeDone; err != nil {
			t.Fatal(err)
		}
		if f.users.has(testEmail) || f.creds.has(testEmail) {
			t.Fatalf("user %v / credential %v survived", f.users.has(testEmail), f.creds.has(testEmail))
		}
	})

	t.Run("reset arrives mid-removal", func(t *testing.T) {
		f := newFixture(t)
		f.users.users[testEmail] = serviceuser.User{Email: testEmail, Role: serviceuser.RoleMember}
		f.creds.passwords[testEmail] = "old"
		entered := make(chan struct{})
		release := make(chan struct{})
		f.users.onRemove = func(email string) {
			close(entered)
			<-release
			delete(f.creds.passwords, email)
		}
		removeDone := make(chan error, 1)
		go func() { removeDone <- f.svc.Remove(ctx, testEmail) }()
		<-entered

		resetDone := make(chan error, 1)
		go func() {
			_, err := f.svc.Reset(ctx, ResetRequest{Email: testEmail, Actor: adminEmail})
			resetDone <- err
		}()
		select {
		case <-resetDone:
			t.Fatal("reset ran while the removal was in progress")
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		if err := <-removeDone; err != nil {
			t.Fatal(err)
		}
		if err := <-resetDone; !errors.Is(err, serviceuser.ErrUserNotFound) {
			t.Fatalf("reset after the removal = %v, want not found", err)
		}
		if f.users.has(testEmail) || f.creds.has(testEmail) || f.creds.issued != 0 {
			t.Fatal("the reset resurrected a credential for a removed user")
		}
	})

	t.Run("many random interleavings", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			f := newFixture(t)
			f.users.users[testEmail] = serviceuser.User{Email: testEmail, Role: serviceuser.RoleMember}
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				_, _ = f.svc.Reset(ctx, ResetRequest{Email: testEmail, Actor: adminEmail})
			}()
			go func() {
				defer wg.Done()
				_ = f.svc.Remove(ctx, testEmail)
			}()
			wg.Wait()
			if f.users.has(testEmail) || f.creds.has(testEmail) {
				t.Fatalf("iteration %d: user %v credential %v", i, f.users.has(testEmail), f.creds.has(testEmail))
			}
		}
	})
}

func TestNoPasswordInAnyErrorOrLogAcrossFailurePaths(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	var errs []error
	collect := func(_ Result, err error) { errs = append(errs, err) }

	// Mail failure whose transport error echoes the password.
	f.sender.sendErr = errors.New("smtp: data: rejected body containing " + testPassword)
	collect(f.svc.Invite(ctx, passwordInvite(true)))
	collect(f.svc.Invite(ctx, passwordInvite(true))) // duplicate
	f.users.users[testEmail] = serviceuser.User{Email: testEmail, Role: serviceuser.RoleMember}
	collect(f.svc.Reset(ctx, ResetRequest{Email: testEmail, Actor: adminEmail, SendEmail: true}))
	collect(f.svc.Reset(ctx, ResetRequest{Email: testEmail, Actor: testEmail}))
	collect(f.svc.Reset(ctx, ResetRequest{Email: localAdmin, Actor: adminEmail}))
	errs = append(errs, f.svc.Remove(ctx, "ghost@example.com"))

	// The transport error above echoes the password on purpose: nothing the
	// service returns or logs may carry it forward.
	f.assertClean(t, errs...)
	for _, err := range errs {
		if err != nil && strings.Contains(err.Error(), "rejected body") {
			t.Fatalf("raw transport error forwarded: %v", err)
		}
	}

	// Credential and rollback failures, including the double failure.
	g := newFixture(t)
	g.creds.issueErr = errors.New("registry unavailable")
	g.users.removeErr = errors.New("users file locked")
	_, err := g.svc.Invite(ctx, passwordInvite(true))
	g.assertClean(t, err)
}
