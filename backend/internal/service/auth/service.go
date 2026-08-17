package auth

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/audit"
)

// UserDirectory is the interface auth needs into the users store. It's
// satisfied by *user.Service; declared here as a small port so auth doesn't
// import user (avoids an import cycle if user ever needs auth).
type UserDirectory interface {
	IsAdmin(ctx context.Context, email string) (bool, error)
	IsRegistered(ctx context.Context, email string) (bool, error)
	// AddBootstrapAdmin promotes email to the first admin. Called only when
	// users.json is empty (no admins exist yet); subsequent sign-ins go
	// through IsRegistered.
	AddBootstrapAdmin(ctx context.Context, email string) error
	FirstAdmin(ctx context.Context) (*UserDirectoryEntry, error)
}

// UserDirectoryEntry is the minimal projection of a single admin the auth
// service exposes via /auth/me. Status.Claimed is set when one exists,
// Status.AdminEmail is its Email. Currently filled from FirstAdmin (the
// oldest user with role=admin) so the login screen can show "server
// administered by …" without leaking the full directory to anonymous
// callers.
type UserDirectoryEntry struct {
	Email string
}

type Service struct {
	users        UserDirectory
	local        *LocalAdminAuthenticator
	google       *GoogleAuthenticator
	baseURL      string
	cookieDomain string
	sessions     *SessionCodec
	audit        audit.Recorder
}

// Option configures optional Service collaborators.
type Option func(*Service)

// WithAudit records sign-ins, the administrator claim, and Google OAuth
// configuration changes to the audit log.
func WithAudit(recorder audit.Recorder) Option {
	return func(s *Service) { s.audit = audit.RecorderOrNop(recorder) }
}

func NormalizeBaseURL(baseURL string) (string, error) {
	if baseURL == "" {
		return "", errors.New("BASE_URL env var required when auth is enabled (e.g. https://remote.example.com)")
	}
	return strings.TrimRight(baseURL, "/"), nil
}

func New(
	ctx context.Context,
	store Store,
	users UserDirectory,
	oauthFactory OAuthProviderFactory,
	baseURL string,
	sessionKey []byte,
	options ...Option,
) (*Service, error) {
	if store == nil {
		return nil, errors.New("auth store is required")
	}
	if oauthFactory == nil {
		return nil, errors.New("OAuth provider factory is required")
	}
	baseURL, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if len(sessionKey) == 0 {
		return nil, errors.New("session key is required")
	}
	localAdmin, err := store.LocalAdmin(ctx)
	if err != nil {
		return nil, err
	}
	local := newLocalAdminAuthenticator(store, users, localAdmin)
	google, err := newGoogleAuthenticator(ctx, store, users, oauthFactory, baseURL, local.isLocalAdmin)
	if err != nil {
		return nil, err
	}
	dummyHash, err := HashPassword("invalid-password-placeholder")
	if err != nil {
		return nil, err
	}
	local.setDummyHash(dummyHash)

	cookieDomain := ""
	if u, err := url.Parse(baseURL); err == nil {
		cookieDomain = u.Hostname()
	}

	service := &Service{
		users:        users,
		local:        local,
		google:       google,
		baseURL:      baseURL,
		cookieDomain: cookieDomain,
		sessions:     newSessionCodec(sessionKey),
		audit:        audit.Nop{},
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service, nil
}

// recordLogin writes one sign-in attempt. The actor is set explicitly because
// a login is exactly the request that has no session to resolve one from, and
// a failed attempt still names the identity that was tried.
func (s *Service) recordLogin(ctx context.Context, method, attemptedEmail string, user User, err error) {
	if s == nil || s.audit == nil {
		return
	}
	action := audit.ActionAuthLoginSuccess
	if err != nil {
		action = audit.ActionAuthLoginFailure
	}
	email := user.Email
	if email == "" {
		email = attemptedEmail
	}
	entry := audit.Result(action, audit.Target{Type: audit.TargetSession}, audit.Meta{"method": method}, err)
	entry.Actor = audit.Actor{Email: audit.NormalizeActorEmail(email), Sub: user.Sub}
	s.audit.Record(ctx, entry)
}

// Audit exposes the recorder to transport-level flows (logout) that have no
// service call of their own to hang an entry on.
func (s *Service) Audit() audit.Recorder {
	if s == nil || s.audit == nil {
		return audit.Nop{}
	}
	return s.audit
}

func (s *Service) BaseURL() string {
	return s.baseURL
}

func (s *Service) CookieDomain() string {
	return s.cookieDomain
}

func (s *Service) AuthCodeURL(state string) (string, error) {
	return s.google.authCodeURL(state)
}

func (s *Service) LoginGoogle(ctx context.Context, code string) (User, error) {
	user, err := s.google.login(ctx, code)
	attempted := ""
	var notInvited NotInvitedError
	if errors.As(err, &notInvited) {
		attempted = notInvited.Email
	}
	s.recordLogin(ctx, "google", attempted, user, err)
	return user, err
}

func (s *Service) ClaimLocalAdmin(ctx context.Context, email, password, authorizedEmail string) (User, error) {
	user, err := s.local.claim(ctx, email, password, authorizedEmail)
	if s.audit != nil {
		entry := audit.Result(
			audit.ActionAuthAdminClaim,
			audit.Target{Type: audit.TargetUser, ID: audit.NormalizeActorEmail(email), Name: audit.NormalizeActorEmail(email)},
			nil,
			err,
		)
		entry.Actor = audit.Actor{
			Email:   audit.NormalizeActorEmail(email),
			Sub:     user.Sub,
			IsAdmin: err == nil,
		}
		s.audit.Record(ctx, entry)
	}
	return user, err
}

func (s *Service) LoginLocal(ctx context.Context, email, password string) (User, error) {
	user, err := s.local.login(email, password)
	s.recordLogin(ctx, "local", email, user, err)
	return user, err
}

func (s *Service) ConfigureGoogleOAuth(ctx context.Context, cfg OAuthConfig) error {
	err := s.google.configure(ctx, cfg)
	if s.audit != nil {
		s.audit.Record(ctx, audit.Result(
			audit.ActionSettingsGoogleOAuth,
			audit.Target{Type: audit.TargetServer, ID: "google-oauth", Name: "Google sign-in"},
			audit.Meta{"clientId": strings.TrimSpace(cfg.GoogleClientID)},
			err,
		))
	}
	return err
}

func (s *Service) GoogleOAuthEnabled() bool {
	return s.google.enabled()
}

func (s *Service) GoogleClientID() string {
	return s.google.clientID()
}

func (s *Service) LocalAdminConfigured() bool {
	return s.local.configured()
}

func (s *Service) IsLocalAdmin(email string) bool {
	return s.local.isLocalAdmin(email)
}

func (s *Service) SignSession(user User) string {
	return s.sessions.sign(user)
}

func (s *Service) CurrentSession(cookieValue string) (*Session, error) {
	if cookieValue == "" {
		return nil, errors.New("missing session cookie")
	}
	session, err := s.sessions.verify(cookieValue)
	if err != nil {
		return nil, err
	}
	// Once the local administrator exists, invalidate any older Google-backed
	// sessions for that email. The owner account must remain password-only;
	// invited users may continue using Google.
	if s.IsLocalAdmin(session.Email) && session.Sub != "local-admin" {
		return nil, ErrLocalAdminPasswordOnly
	}
	return session, nil
}

func (s *Service) IsAdmin(ctx context.Context, email string) (bool, error) {
	if s.IsLocalAdmin(email) {
		return true, nil
	}
	if s.users == nil {
		return false, nil
	}
	return s.users.IsAdmin(ctx, email)
}

// IsRegistered returns true if email has a row in the users store. Used by
// the API middleware so members (not just admins) can reach /api/*.
func (s *Service) IsRegistered(ctx context.Context, email string) (bool, error) {
	if s.IsLocalAdmin(email) {
		return true, nil
	}
	if s.users == nil {
		return false, nil
	}
	return s.users.IsRegistered(ctx, email)
}

func (s *Service) Status(ctx context.Context, cookieValue string) Status {
	status := Status{
		LocalAdminConfigured: s.LocalAdminConfigured(),
		GoogleOAuthEnabled:   s.GoogleOAuthEnabled(),
		GoogleClientID:       s.GoogleClientID(),
	}
	if email, ok := s.local.adminEmail(); ok {
		status.Claimed = true
		status.AdminEmail = email
	}
	if !status.Claimed && s.users != nil {
		if first, _ := s.users.FirstAdmin(ctx); first != nil {
			status.Claimed = true
			status.AdminEmail = first.Email
		}
	}

	session, err := s.CurrentSession(cookieValue)
	if err != nil {
		return status
	}
	status.Authenticated = true
	status.Email = session.Email
	status.Sub = session.Sub
	status.IsAdmin, _ = s.IsAdmin(ctx, session.Email)
	status.IsRegistered, _ = s.IsRegistered(ctx, session.Email)
	return status
}

func SessionDuration() time.Duration {
	return sessionDuration
}
