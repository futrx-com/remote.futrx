package service

import (
	"context"
	"errors"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/googleoauth"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
)

// userDirectoryAdapter wraps *user.Service to satisfy auth.UserDirectory.
// AddBootstrapAdmin is the one method the regular user service does not expose
// directly because the bootstrap path has no adding administrator.
type userDirectoryAdapter struct {
	users *serviceuser.Service
}

func (a userDirectoryAdapter) IsAdmin(ctx context.Context, email string) (bool, error) {
	return a.users.IsAdmin(ctx, email)
}

func (a userDirectoryAdapter) IsRegistered(ctx context.Context, email string) (bool, error) {
	return a.users.IsRegistered(ctx, email)
}

func (a userDirectoryAdapter) AddBootstrapAdmin(ctx context.Context, email string) error {
	_, err := a.users.Add(ctx, email, serviceuser.RoleAdmin, "")
	return err
}

func (a userDirectoryAdapter) FirstAdmin(ctx context.Context) (*serviceauth.UserDirectoryEntry, error) {
	list, err := a.users.List(ctx)
	if err != nil {
		return nil, err
	}
	var oldest *serviceuser.User
	for i := range list {
		user := &list[i]
		if user.Role != serviceuser.RoleAdmin {
			continue
		}
		if oldest == nil || user.AddedAt < oldest.AddedAt {
			oldest = user
		}
	}
	if oldest == nil {
		return nil, nil
	}
	return &serviceauth.UserDirectoryEntry{Email: oldest.Email}, nil
}

// NewSetupTokenIssuer composes the operator-only setup-token use case from
// its narrow persistence and user-directory dependencies.
func NewSetupTokenIssuer(
	ctx context.Context,
	store serviceauth.SetupTokenIssuerStore,
	users *serviceuser.Service,
	ttl time.Duration,
) (*serviceauth.SetupTokenIssuer, error) {
	if users == nil {
		return nil, errors.New("user directory is required")
	}
	return serviceauth.NewSetupTokenIssuer(ctx, store, userDirectoryAdapter{users: users}, ttl)
}

// newAuth composes the complete runtime auth service. Operator commands use
// NewSetupTokenIssuer instead so they do not initialize unrelated auth
// capabilities or create a session key.
func newAuth(
	ctx context.Context,
	store AuthStore,
	users *serviceuser.Service,
	baseURL string,
	twoFactor serviceauth.TwoFactorStore,
	sessionRegistry serviceauth.SessionRegistryStore,
	options AuthOptions,
) (*serviceauth.Service, error) {
	if store == nil {
		return nil, errors.New("authentication store is required")
	}
	baseURL, err := serviceauth.NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	sessionKey, err := store.SessionKey(ctx)
	if err != nil {
		return nil, err
	}

	var directory serviceauth.UserDirectory
	if users != nil {
		directory = userDirectoryAdapter{users: users}
	}
	return serviceauth.New(
		ctx,
		store,
		directory,
		func(clientID, clientSecret, redirectURL string) serviceauth.OAuthProvider {
			return googleoauth.New(clientID, clientSecret, redirectURL)
		},
		baseURL,
		sessionKey,
		twoFactor,
		sessionRegistry,
		serviceauth.Options{
			PendingLoginTTL:     options.PendingLoginTTL,
			EnrollmentTTL:       options.EnrollmentTTL,
			RecoveryCodeCount:   options.RecoveryCodeCount,
			SessionHistoryLimit: options.SessionHistoryLimit,
			SetupTokenTTL:       options.SetupTokenTTL,
		},
	)
}
