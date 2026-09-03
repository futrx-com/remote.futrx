package auth

import "context"

type OAuthConfigStore interface {
	OAuthConfig(context.Context) (OAuthConfig, error)
	SaveOAuthConfig(context.Context, OAuthConfig) error
}

type LocalAdminStore interface {
	LocalAdmin(context.Context) (*LocalAdminCredential, error)
	CreateLocalAdmin(context.Context, LocalAdminCredential) error
	// DeleteLocalAdmin removes only the credential that exactly matches the
	// expected value. It is reserved for compensating a failed claim.
	DeleteLocalAdmin(context.Context, LocalAdminCredential) error
}

type Store interface {
	OAuthConfigStore
	LocalAdminStore
	SessionKey(context.Context) ([]byte, error)
}

// TwoFactorStore persists one TwoFactorRecord per enrolled account, keyed by
// email. Get returns (nil, nil) when the account has never enrolled - that
// absence is the correct "2FA not enabled" state, not an error.
type TwoFactorStore interface {
	Get(ctx context.Context, email string) (*TwoFactorRecord, error)
	Save(ctx context.Context, email string, record TwoFactorRecord) error
	Delete(ctx context.Context, email string) error
}

// SessionRegistryStore persists one SessionRegistryRecord per account, keyed
// by email. Get returns (nil, nil) when the account has never touched any of
// the three SecurityPreferences flags - that absence is the correct
// "nothing enabled" state, not an error.
type SessionRegistryStore interface {
	Get(ctx context.Context, email string) (*SessionRegistryRecord, error)
	Save(ctx context.Context, email string, record SessionRegistryRecord) error
	Delete(ctx context.Context, email string) error
}

type OAuthProvider interface {
	AuthCodeURL(state string) string
	ExchangeUser(ctx context.Context, code string) (User, error)
}

type OAuthProviderFactory func(clientID, clientSecret, redirectURL string) OAuthProvider
