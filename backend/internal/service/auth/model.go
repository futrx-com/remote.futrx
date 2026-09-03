package auth

import "errors"

const (
	SessionCookieName = "remote_session"
	StateCookieName   = "remote_oauth_state"
	// PendingTwoFactorCookieName carries the short-lived pending-login token
	// between a completed first factor (password/Google) and the 2FA challenge
	// endpoint. Its lifetime is PendingTwoFactorDuration.
	PendingTwoFactorCookieName = "remote_2fa_pending"
)

var ErrOAuthConfigNotFound = errors.New("oauth config not found")

var (
	ErrLocalAdminAlreadyClaimed    = errors.New("local admin is already configured")
	ErrLocalAdminCredentialChanged = errors.New("local admin credential no longer matches")
	ErrAdminClaimUnauthorized      = errors.New("an existing administrator must authorize local password setup")
	ErrInvalidCredentials          = errors.New("invalid email or password")
	ErrPasswordTooShort            = errors.New("password must be at least 12 characters")
	ErrPasswordTooLong             = errors.New("password is too long")
	ErrInvalidOAuthConfig          = errors.New("Google OAuth client ID and client secret are required")
	ErrGoogleOAuthDisabled         = errors.New("Google sign-in is not configured")
	ErrLocalAdminPasswordOnly      = errors.New("the local administrator must sign in with a password")
)

type OAuthConfig struct {
	GoogleClientID     string `json:"googleClientId"`
	GoogleClientSecret string `json:"googleClientSecret"`
}

type LocalAdminCredential struct {
	Email        string `json:"email"`
	PasswordHash string `json:"passwordHash"`
}

type User struct {
	Email   string
	Sub     string
	Name    string
	Picture string
}

type Session struct {
	Email string `json:"email"`
	Sub   string `json:"sub"`
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
	// SID identifies this specific issued session. Empty for any session
	// signed before this field existed (those still verify fine) and for
	// accounts that have never turned on "single active session" or
	// "sign-in history" (see SecurityPreferences) - in both cases there is
	// nothing to look up in the session registry.
	SID string `json:"sid,omitempty"`
}

type Status struct {
	Authenticated        bool           `json:"authenticated"`
	Claimed              bool           `json:"claimed"`
	LocalAdminConfigured bool           `json:"localAdminConfigured"`
	GoogleOAuthEnabled   bool           `json:"googleOAuthEnabled"`
	GoogleClientID       string         `json:"googleClientId,omitempty"`
	AdminEmail           string         `json:"adminEmail,omitempty"`
	Email                string         `json:"email,omitempty"`
	Sub                  string         `json:"sub,omitempty"`
	IsAdmin              bool           `json:"isAdmin,omitempty"`
	IsRegistered         bool           `json:"isRegistered,omitempty"`
	SecurityAlert        *SecurityAlert `json:"securityAlert,omitempty"`
}

// SignInMethod records which factor(s) completed a login. The recovery-code
// variants are the precise, buildable signal for "someone bypassed the
// authenticator app" that the recovery-code alert fires on.
type SignInMethod string

const (
	SignInMethodPassword             SignInMethod = "password"
	SignInMethodGoogle               SignInMethod = "google"
	SignInMethodPasswordTOTP         SignInMethod = "password+totp"
	SignInMethodGoogleTOTP           SignInMethod = "google+totp"
	SignInMethodPasswordRecoveryCode SignInMethod = "password+recovery-code"
	SignInMethodGoogleRecoveryCode   SignInMethod = "google+recovery-code"
)

func (m SignInMethod) usedRecoveryCode() bool {
	return m == SignInMethodPasswordRecoveryCode || m == SignInMethodGoogleRecoveryCode
}

// TwoFactorRecord is the persisted state of one account's TOTP enrollment:
// the raw secret (plaintext at rest - see plan Deviations, it must be
// recoverable to recompute HMAC codes) and the remaining recovery-code
// hashes.
type TwoFactorRecord struct {
	Secret             []byte   `json:"secret"`
	RecoveryCodeHashes []string `json:"recoveryCodeHashes"`
	EnabledAt          int64    `json:"enabledAt"`
	// LastUsedTOTPCounter is the time-step counter of the most recent code
	// accepted for a sign-in, so the same code cannot be replayed inside its
	// validity window. Zero for records written before replay tracking
	// existed, which simply means the next code is unconstrained.
	LastUsedTOTPCounter uint64 `json:"lastUsedTotpCounter,omitempty"`
}

// SessionRecord is one entry in an account's bounded sign-in history.
type SessionRecord struct {
	SID       string       `json:"sid"`
	Method    SignInMethod `json:"method"`
	IP        string       `json:"ip"`
	UserAgent string       `json:"userAgent"`
	IssuedAt  int64        `json:"issuedAt"`
}

// SessionHistory is the bounded (newest-first), bounded-size list of past
// sign-ins for one account. Only populated while HistoryEnabled is on.
type SessionHistory struct {
	Entries []SessionRecord `json:"entries"`
}

// SecurityAlert is set when a sign-in used a recovery code while
// RecoveryCodeAlertEnabled is on, and cleared on acknowledgement.
type SecurityAlert struct {
	Method       SignInMethod `json:"method"`
	IP           string       `json:"ip"`
	UserAgent    string       `json:"userAgent"`
	OccurredAt   int64        `json:"occurredAt"`
	Acknowledged bool         `json:"acknowledged"`
}

// SecurityPreferences holds the three independent, per-account toggles this
// plan introduces alongside 2FA. Each defaults to false (off). Single active
// session and history are meaningful with or without 2FA; the alert flag is
// only settable while 2FA is enabled, since it fires on recovery-code use.
type SecurityPreferences struct {
	SingleSessionEnabled     bool `json:"singleSessionEnabled"`
	HistoryEnabled           bool `json:"historyEnabled"`
	RecoveryCodeAlertEnabled bool `json:"recoveryCodeAlertEnabled"`
}

// SessionRegistryRecord is the per-account file SessionRegistryStore
// persists: preferences, the currently active session id (single-session
// enforcement), bounded history, and any pending alert.
type SessionRegistryRecord struct {
	Preferences     SecurityPreferences `json:"preferences"`
	ActiveSessionID string              `json:"activeSessionId,omitempty"`
	History         SessionHistory      `json:"history"`
	Alert           *SecurityAlert      `json:"alert,omitempty"`
}

// SecuritySummary is the aggregate view the Security settings tab (and its
// GET /api/me/security endpoint) renders: 2FA status, the three independent
// SecurityPreferences flags, sign-in history, and any pending alert.
type SecuritySummary struct {
	TwoFactorEnabled         bool            `json:"twoFactorEnabled"`
	RecoveryCodesRemaining   int             `json:"recoveryCodesRemaining"`
	SingleSessionEnabled     bool            `json:"singleSessionEnabled"`
	HistoryEnabled           bool            `json:"historyEnabled"`
	RecoveryCodeAlertEnabled bool            `json:"recoveryCodeAlertEnabled"`
	Sessions                 []SessionRecord `json:"sessions"`
	SecurityAlert            *SecurityAlert  `json:"securityAlert,omitempty"`
}

// ClaimedError is returned in the legacy single-admin path when a second
// user tries to sign in before the users-store is wired up.

// NotInvitedError is returned by Login when a Google OAuth flow succeeded
// but the resulting email is not in the users store. Surfaced to the
// frontend so the login screen can show a friendly "ask an admin" message.
type NotInvitedError struct {
	Email string
}

func (e NotInvitedError) Error() string {
	return "not invited - ask an admin to add your email (" + e.Email + ")"
}
