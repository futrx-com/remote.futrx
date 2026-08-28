package auth

import (
	"context"
	"errors"
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

const pendingLoginTTL = 5 * time.Minute

// pendingLoginDomain separates the half-authenticated pending-login token
// from a real Session. Both are signed with the session key and their JSON
// overlaps, so without this a pending token decodes as a valid session and
// the second factor can be skipped entirely.
const pendingLoginDomain = "pending-login/v1"

// LoginResult is returned by the Complete*Login methods: either a login
// completed outright (CookieValue set) or it needs a second factor
// (PendingToken set, to be presented back to CompleteTwoFactorChallenge).
type LoginResult struct {
	Completed    bool
	CookieValue  string
	PendingToken string
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
		users:             users,
		local:             local,
		google:            google,
		baseURL:           baseURL,
		cookieDomain:      cookieDomain,
		codec:             newSessionCodec(sessionKey),
		twoFactor:         newTwoFactorAuthenticator(twoFactorStore, "remote.futrx", sessionKey),
		registry:          newSessionRegistry(sessionRegistryStore),
		pendingLoginCodec: newSignedPayload[pendingLogin](sessionKey, pendingLoginDomain),
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
			Exp:    time.Now().Add(pendingLoginTTL).Unix(),
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

// RevokeSession clears email's active session id (used on logout), a no-op
// for an account with no session registry record.
func (s *Service) RevokeSession(ctx context.Context, email string) error {
	return s.registry.Revoke(ctx, email)
}

// TwoFactorEnabled reports whether email has completed TOTP enrollment.
func (s *Service) TwoFactorEnabled(ctx context.Context, email string) bool {
	return s.twoFactor.Enabled(ctx, email)
}

// BeginTwoFactorEnrollment starts TOTP enrollment for email; see
// twoFactorAuthenticator.BeginEnrollment.
func (s *Service) BeginTwoFactorEnrollment(ctx context.Context, email string) (enrollmentToken, secretBase32, otpauthURL string, err error) {
	return s.twoFactor.BeginEnrollment(ctx, email)
}

// ConfirmTwoFactorEnrollment completes TOTP enrollment; see
// twoFactorAuthenticator.ConfirmEnrollment.
func (s *Service) ConfirmTwoFactorEnrollment(ctx context.Context, expectedEmail, enrollmentToken, code string) (recoveryCodes []string, email string, err error) {
	return s.twoFactor.ConfirmEnrollment(ctx, expectedEmail, enrollmentToken, code)
}

// DisableTwoFactor removes email's 2FA enrollment after verifying proof of
// possession; see twoFactorAuthenticator.Disable.
func (s *Service) DisableTwoFactor(ctx context.Context, email, code string) error {
	return s.twoFactor.Disable(ctx, email, code)
}

// RegenerateRecoveryCodes replaces email's recovery codes; see
// twoFactorAuthenticator.RegenerateRecoveryCodes.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, email, code string) ([]string, error) {
	return s.twoFactor.RegenerateRecoveryCodes(ctx, email, code)
}

// SecurityPreferences returns email's current SecurityPreferences.
func (s *Service) SecurityPreferences(ctx context.Context, email string) (SecurityPreferences, error) {
	return s.registry.Preferences(ctx, email)
}

// SetSecurityPreferences overwrites email's SecurityPreferences.
func (s *Service) SetSecurityPreferences(ctx context.Context, email string, prefs SecurityPreferences) error {
	return s.registry.SetPreferences(ctx, email, prefs)
}

// AckSecurityAlert clears email's pending SecurityAlert, if any.
func (s *Service) AckSecurityAlert(ctx context.Context, email string) error {
	return s.registry.AckAlert(ctx, email)
}

// SecuritySummary aggregates 2FA status, SecurityPreferences, sign-in
// history, and any pending alert for the Security settings tab.
func (s *Service) SecuritySummary(ctx context.Context, email string) (SecuritySummary, error) {
	prefs, err := s.registry.Preferences(ctx, email)
	if err != nil {
		return SecuritySummary{}, err
	}
	history, err := s.registry.History(ctx, email)
	if err != nil {
		return SecuritySummary{}, err
	}
	alert, err := s.registry.PendingAlert(ctx, email)
	if err != nil {
		return SecuritySummary{}, err
	}
	return SecuritySummary{
		TwoFactorEnabled:         s.twoFactor.Enabled(ctx, email),
		RecoveryCodesRemaining:   s.twoFactor.RecoveryCodesRemaining(ctx, email),
		SingleSessionEnabled:     prefs.SingleSessionEnabled,
		HistoryEnabled:           prefs.HistoryEnabled,
		RecoveryCodeAlertEnabled: prefs.RecoveryCodeAlertEnabled,
		Sessions:                 history.Entries,
		SecurityAlert:            alert,
	}, nil
}

// ReissueTrackedSession re-signs a new session for an already-authenticated
// user, going through the same IssueSession path a fresh login would. Used
// right after a Security-tab change (enabling 2FA, single-session, etc.) so
// the browser that just made the change is immediately recognized as the
// account's active/tracked session, instead of waiting for its next login.
// The SignInMethod recorded is inferred from the session's Sub (local-admin
// implies password; anything else implies Google) since this isn't a fresh
// credential check.
func (s *Service) ReissueTrackedSession(ctx context.Context, user User, ip, userAgent string) (string, error) {
	method := SignInMethodGoogle
	if user.Sub == "local-admin" {
		method = SignInMethodPassword
	}
	return s.IssueSession(ctx, user, method, ip, userAgent)
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
