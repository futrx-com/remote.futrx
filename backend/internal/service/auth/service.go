package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
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

var (
	ErrSessionSuperseded   = errors.New("session superseded by a newer sign-in")
	ErrInvalidPendingLogin = errors.New("invalid or expired pending login")
)

// pendingLogin is the signed, stateless payload carried between
// CompletePasswordLogin/CompleteGoogleLogin (once credentials check out but
// before the second factor is checked) and CompleteTwoFactorChallenge.
type pendingLogin struct {
	Email  string       `json:"email"`
	Sub    string       `json:"sub"`
	Method SignInMethod `json:"method"`
	Exp    int64        `json:"exp"`
}

func (p pendingLogin) expired(now time.Time) bool {
	return now.Unix() > p.Exp
}

// LoginResult is returned by the Complete*Login methods: either a login
// completed outright (CookieValue set) or it needs a second factor
// (PendingToken set, to be presented back to CompleteTwoFactorChallenge).
type LoginResult struct {
	Completed    bool
	CookieValue  string
	PendingToken string
}

// Options are application-wide account security policies supplied by the
// composition root. Cryptographic protocol parameters remain package-owned.
type Options struct {
	PendingLoginTTL     time.Duration
	EnrollmentTTL       time.Duration
	RecoveryCodeCount   int
	SessionHistoryLimit int
}

func (o Options) validate() error {
	if o.PendingLoginTTL <= 0 {
		return errors.New("pending login TTL must be positive")
	}
	if o.EnrollmentTTL <= 0 {
		return errors.New("enrollment TTL must be positive")
	}
	if o.RecoveryCodeCount <= 0 {
		return errors.New("recovery code count must be positive")
	}
	if o.SessionHistoryLimit <= 0 {
		return errors.New("session history limit must be positive")
	}
	return nil
}

type Service struct {
	users             UserDirectory
	local             *LocalAdminAuthenticator
	google            *GoogleAuthenticator
	baseURL           string
	cookieDomain      string
	codec             *sessionCodec
	twoFactor         *twoFactorAuthenticator
	registry          *sessionRegistry
	pendingLoginCodec signedPayload[pendingLogin]
	pendingLoginTTL   time.Duration
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
	twoFactorStore TwoFactorStore,
	sessionRegistryStore SessionRegistryStore,
	options Options,
) (*Service, error) {
	if store == nil {
		return nil, errors.New("auth store is required")
	}
	if oauthFactory == nil {
		return nil, errors.New("OAuth provider factory is required")
	}
	if twoFactorStore == nil {
		return nil, errors.New("two-factor store is required")
	}
	if sessionRegistryStore == nil {
		return nil, errors.New("session registry store is required")
	}
	if err := options.validate(); err != nil {
		return nil, fmt.Errorf("auth options: %w", err)
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
		codec:        newSessionCodec(sessionKey),
		twoFactor: newTwoFactorAuthenticator(
			twoFactorStore,
			"remote.futrx",
			sessionKey,
			options.EnrollmentTTL,
			options.RecoveryCodeCount,
		),
		registry:          newSessionRegistry(sessionRegistryStore, options.SessionHistoryLimit),
		pendingLoginCodec: newPendingLoginPayload(sessionKey),
		pendingLoginTTL:   options.PendingLoginTTL,
	}
	return service, nil
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
	return s.google.login(ctx, code)
}

func (s *Service) ClaimLocalAdmin(ctx context.Context, email, password, authorizedEmail string) (User, error) {
	return s.local.claim(ctx, email, password, authorizedEmail)
}

func (s *Service) LoginLocal(_ context.Context, email, password string) (User, error) {
	return s.local.login(email, password)
}

func (s *Service) ConfigureGoogleOAuth(ctx context.Context, cfg OAuthConfig) error {
	return s.google.configure(ctx, cfg)
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

// IssueSession signs a new session for user, first consulting the account's
// SecurityPreferences: if any of the three flags (single-session, history,
// recovery-code alert) is on, it registers the sign-in with the session registry
// and embeds the resulting session id; otherwise it behaves exactly like
// SignSession (no registry write, no per-request registry lookup cost for
// accounts that opt into nothing).
func (s *Service) IssueSession(ctx context.Context, user User, method SignInMethod, ip, userAgent string) (string, error) {
	prefs, err := s.registry.Preferences(ctx, user.Email)
	if err != nil {
		return "", err
	}
	sid := ""
	if prefs.SingleSessionEnabled || prefs.HistoryEnabled || prefs.RecoveryCodeAlertEnabled {
		sid, err = s.registry.IssueForAccount(ctx, user.Email, method, ip, userAgent)
		if err != nil {
			return "", err
		}
	}
	return s.codec.sign(user, sid), nil
}

// CompletePasswordLogin verifies credentials and either issues a session
// outright (2FA off for this account) or returns a pending token that must
// be completed via CompleteTwoFactorChallenge.
func (s *Service) CompletePasswordLogin(ctx context.Context, email, password, ip, userAgent string) (LoginResult, error) {
	user, err := s.LoginLocal(ctx, email, password)
	if err != nil {
		return LoginResult{}, err
	}
	return s.completeLogin(ctx, user, SignInMethodPassword, ip, userAgent)
}

// CompleteGoogleLogin is the Google analogue of CompletePasswordLogin.
func (s *Service) CompleteGoogleLogin(ctx context.Context, code, ip, userAgent string) (LoginResult, error) {
	user, err := s.LoginGoogle(ctx, code)
	if err != nil {
		return LoginResult{}, err
	}
	return s.completeLogin(ctx, user, SignInMethodGoogle, ip, userAgent)
}

func (s *Service) completeLogin(ctx context.Context, user User, method SignInMethod, ip, userAgent string) (LoginResult, error) {
	if s.twoFactor.Enabled(ctx, user.Email) {
		token := s.pendingLoginCodec.sign(pendingLogin{
			Email:  user.Email,
			Sub:    user.Sub,
			Method: method,
			Exp:    time.Now().Add(s.pendingLoginTTL).Unix(),
		})
		return LoginResult{Completed: false, PendingToken: token}, nil
	}
	cookieValue, err := s.IssueSession(ctx, user, method, ip, userAgent)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Completed: true, CookieValue: cookieValue}, nil
}

// CompleteTwoFactorChallenge verifies a pending login's second factor and,
// on success, issues the real session with the combined SignInMethod
// (e.g. "password+totp", "google+recovery-code").
func (s *Service) CompleteTwoFactorChallenge(ctx context.Context, pendingToken, code, ip, userAgent string) (LoginResult, error) {
	pending, err := s.pendingLoginCodec.verify(pendingToken)
	if err != nil {
		return LoginResult{}, ErrInvalidPendingLogin
	}
	usedRecoveryCode, err := s.twoFactor.VerifyChallenge(ctx, pending.Email, code)
	if err != nil {
		return LoginResult{}, err
	}
	method := combineSignInMethod(pending.Method, usedRecoveryCode)
	cookieValue, err := s.IssueSession(ctx, User{Email: pending.Email, Sub: pending.Sub}, method, ip, userAgent)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Completed: true, CookieValue: cookieValue}, nil
}

func combineSignInMethod(base SignInMethod, usedRecoveryCode bool) SignInMethod {
	switch base {
	case SignInMethodPassword:
		if usedRecoveryCode {
			return SignInMethodPasswordRecoveryCode
		}
		return SignInMethodPasswordTOTP
	case SignInMethodGoogle:
		if usedRecoveryCode {
			return SignInMethodGoogleRecoveryCode
		}
		return SignInMethodGoogleTOTP
	default:
		return base
	}
}

func (s *Service) CurrentSession(ctx context.Context, cookieValue string) (*Session, error) {
	if cookieValue == "" {
		return nil, errors.New("missing session cookie")
	}
	session, err := s.codec.verify(cookieValue)
	if err != nil {
		return nil, err
	}
	// Once the local administrator exists, invalidate any older Google-backed
	// sessions for that email. The owner account must remain password-only;
	// invited users may continue using Google.
	if s.IsLocalAdmin(session.Email) && session.Sub != "local-admin" {
		return nil, ErrLocalAdminPasswordOnly
	}
	// Single active session is one more account-scoped rule here, consulted
	// only when the account has independently turned SingleSessionEnabled on
	// (sessionRegistry.IsActive treats every session as active otherwise).
	if !s.registry.IsActive(ctx, session.Email, session.SID) {
		return nil, ErrSessionSuperseded
	}
	return session, nil
}

// RevokeSession replaces email's active session id with an unissued id (used
// on logout), a no-op for an account with no session registry record.
func (s *Service) RevokeSession(ctx context.Context, email string) error {
	return s.registry.Revoke(ctx, email)
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

	session, err := s.CurrentSession(ctx, cookieValue)
	if err != nil {
		return status
	}
	status.Authenticated = true
	status.Email = session.Email
	status.Sub = session.Sub
	status.IsAdmin, _ = s.IsAdmin(ctx, session.Email)
	status.IsRegistered, _ = s.IsRegistered(ctx, session.Email)
	if prefs, _ := s.registry.Preferences(ctx, session.Email); prefs.RecoveryCodeAlertEnabled {
		if alert, _ := s.registry.PendingAlert(ctx, session.Email); alert != nil {
			status.SecurityAlert = alert
		}
	}
	return status
}

func SessionDuration() time.Duration {
	return sessionDuration
}

func (s *Service) PendingTwoFactorDuration() time.Duration {
	return s.pendingLoginTTL
}
