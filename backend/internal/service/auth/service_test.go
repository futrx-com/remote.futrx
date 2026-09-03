package auth

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
)

func authTestOptions() Options {
	return Options{
		PendingLoginTTL:     5 * time.Minute,
		EnrollmentTTL:       10 * time.Minute,
		RecoveryCodeCount:   10,
		SessionHistoryLimit: 20,
	}
}

type authTestStore struct {
	local     *LocalAdminCredential
	oauth     OAuthConfig
	key       []byte
	deleteErr error
}

func (s *authTestStore) OAuthConfig(context.Context) (OAuthConfig, error) {
	if s.oauth.GoogleClientID == "" {
		return OAuthConfig{}, ErrOAuthConfigNotFound
	}
	return s.oauth, nil
}
func (s *authTestStore) SaveOAuthConfig(_ context.Context, cfg OAuthConfig) error {
	s.oauth = cfg
	return nil
}
func (s *authTestStore) LocalAdmin(context.Context) (*LocalAdminCredential, error) {
	if s.local == nil {
		return nil, nil
	}
	copy := *s.local
	return &copy, nil
}
func (s *authTestStore) CreateLocalAdmin(_ context.Context, credential LocalAdminCredential) error {
	if s.local != nil {
		return ErrLocalAdminAlreadyClaimed
	}
	copy := credential
	s.local = &copy
	return nil
}
func (s *authTestStore) DeleteLocalAdmin(ctx context.Context, expected LocalAdminCredential) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.deleteErr != nil {
		return s.deleteErr
	}
	if s.local == nil {
		return nil
	}
	if *s.local != expected {
		return ErrLocalAdminCredentialChanged
	}
	s.local = nil
	return nil
}
func (s *authTestStore) SessionKey(context.Context) ([]byte, error) { return s.key, nil }

type authTestTwoFactorStore struct {
	records map[string]TwoFactorRecord
}

func newAuthTestTwoFactorStore() *authTestTwoFactorStore {
	return &authTestTwoFactorStore{records: map[string]TwoFactorRecord{}}
}

func (s *authTestTwoFactorStore) Get(_ context.Context, email string) (*TwoFactorRecord, error) {
	record, ok := s.records[normalizeEmail(email)]
	if !ok {
		return nil, nil
	}
	copy := record
	return &copy, nil
}
func (s *authTestTwoFactorStore) Save(_ context.Context, email string, record TwoFactorRecord) error {
	s.records[normalizeEmail(email)] = record
	return nil
}
func (s *authTestTwoFactorStore) Delete(_ context.Context, email string) error {
	delete(s.records, normalizeEmail(email))
	return nil
}

type authTestSessionRegistryStore struct {
	records map[string]SessionRegistryRecord
}

func newAuthTestSessionRegistryStore() *authTestSessionRegistryStore {
	return &authTestSessionRegistryStore{records: map[string]SessionRegistryRecord{}}
}

func (s *authTestSessionRegistryStore) Get(_ context.Context, email string) (*SessionRegistryRecord, error) {
	record, ok := s.records[normalizeEmail(email)]
	if !ok {
		return nil, nil
	}
	copy := record
	return &copy, nil
}
func (s *authTestSessionRegistryStore) Save(_ context.Context, email string, record SessionRegistryRecord) error {
	s.records[normalizeEmail(email)] = record
	return nil
}
func (s *authTestSessionRegistryStore) Delete(_ context.Context, email string) error {
	delete(s.records, normalizeEmail(email))
	return nil
}

type authTestUsers struct {
	roles                map[string]bool
	isRegisteredErr      error
	addBootstrapAdminErr error
	cancelOnIsRegistered context.CancelFunc
}

func newAuthTestUsers() *authTestUsers { return &authTestUsers{roles: make(map[string]bool)} }
func (u *authTestUsers) IsAdmin(_ context.Context, email string) (bool, error) {
	return u.roles[normalizeEmail(email)], nil
}
func (u *authTestUsers) IsRegistered(_ context.Context, email string) (bool, error) {
	if u.cancelOnIsRegistered != nil {
		u.cancelOnIsRegistered()
		u.cancelOnIsRegistered = nil
	}
	if u.isRegisteredErr != nil {
		return false, u.isRegisteredErr
	}
	_, ok := u.roles[normalizeEmail(email)]
	return ok, nil
}
func (u *authTestUsers) AddBootstrapAdmin(_ context.Context, email string) error {
	if u.addBootstrapAdminErr != nil {
		return u.addBootstrapAdminErr
	}
	u.roles[normalizeEmail(email)] = true
	return nil
}
func (u *authTestUsers) FirstAdmin(context.Context) (*UserDirectoryEntry, error) {
	admins := make([]string, 0)
	for email, admin := range u.roles {
		if admin {
			admins = append(admins, email)
		}
	}
	if len(admins) == 0 {
		return nil, nil
	}
	sort.Strings(admins)
	return &UserDirectoryEntry{Email: admins[0]}, nil
}

type authTestOAuth struct{ user User }

func (o authTestOAuth) AuthCodeURL(state string) string                    { return "https://google.test/?state=" + state }
func (o authTestOAuth) ExchangeUser(context.Context, string) (User, error) { return o.user, nil }

func newAuthTestService(t *testing.T, store *authTestStore, users *authTestUsers, googleUser User) *Service {
	return newAuthTestServiceWithOptions(t, store, users, googleUser, authTestOptions())
}

func newAuthTestServiceWithOptions(
	t *testing.T,
	store *authTestStore,
	users *authTestUsers,
	googleUser User,
	options Options,
) *Service {
	t.Helper()
	service, err := New(
		context.Background(),
		store,
		users,
		func(string, string, string) OAuthProvider { return authTestOAuth{user: googleUser} },
		"https://remote.example.com",
		[]byte("test-session-key"),
		newAuthTestTwoFactorStore(),
		newAuthTestSessionRegistryStore(),
		options,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return service
}

func TestNewAppliesPendingLoginTTL(t *testing.T) {
	options := authTestOptions()
	options.PendingLoginTTL = 42 * time.Second
	service := newAuthTestServiceWithOptions(t, &authTestStore{}, newAuthTestUsers(), User{}, options)
	if got := service.PendingTwoFactorDuration(); got != 42*time.Second {
		t.Fatalf("pending two-factor duration = %s, want 42s", got)
	}
}

func TestLocalAdminClaimAndLogin(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{})

	claimed, err := service.ClaimLocalAdmin(context.Background(), "Admin@Example.com", "correct horse battery staple", "")
	if err != nil {
		t.Fatalf("ClaimLocalAdmin: %v", err)
	}
	if claimed.Email != "admin@example.com" || !service.LocalAdminConfigured() {
		t.Fatalf("claimed admin = %#v", claimed)
	}
	if store.local == nil || strings.Contains(store.local.PasswordHash, "correct horse") {
		t.Fatal("local credential did not store only a password hash")
	}
	if _, err := service.LoginLocal(context.Background(), "admin@example.com", "correct horse battery staple"); err != nil {
		t.Fatalf("LoginLocal: %v", err)
	}
	if _, err := service.LoginLocal(context.Background(), "admin@example.com", "wrong password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v", err)
	}
	if _, err := service.ClaimLocalAdmin(context.Background(), "other@example.com", "another secure password", ""); !errors.Is(err, ErrLocalAdminAlreadyClaimed) {
		t.Fatalf("second claim error = %v", err)
	}
}

func TestLocalAdminClaimRollsBackAfterDirectoryLookupFailure(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{})
	ctx, cancel := context.WithCancel(context.Background())
	users.cancelOnIsRegistered = cancel
	users.isRegisteredErr = context.Canceled

	if _, err := service.ClaimLocalAdmin(ctx, "admin@example.com", "correct horse battery staple", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("ClaimLocalAdmin error = %v, want context cancellation", err)
	}
	if store.local != nil || service.LocalAdminConfigured() {
		t.Fatal("failed claim left a durable or cached local credential")
	}

	users.isRegisteredErr = nil
	if _, err := service.ClaimLocalAdmin(context.Background(), "admin@example.com", "correct horse battery staple", ""); err != nil {
		t.Fatalf("retry ClaimLocalAdmin: %v", err)
	}
}

func TestLocalAdminClaimRollsBackAfterBootstrapFailure(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	bootstrapErr := errors.New("write users directory")
	users.addBootstrapAdminErr = bootstrapErr
	service := newAuthTestService(t, store, users, User{})

	if _, err := service.ClaimLocalAdmin(context.Background(), "admin@example.com", "correct horse battery staple", ""); !errors.Is(err, bootstrapErr) {
		t.Fatalf("ClaimLocalAdmin error = %v, want %v", err, bootstrapErr)
	}
	if store.local != nil || service.LocalAdminConfigured() {
		t.Fatal("failed bootstrap left a durable or cached local credential")
	}
	if len(users.roles) != 0 {
		t.Fatalf("failed bootstrap created users: %#v", users.roles)
	}

	users.addBootstrapAdminErr = nil
	if _, err := service.ClaimLocalAdmin(context.Background(), "admin@example.com", "correct horse battery staple", ""); err != nil {
		t.Fatalf("retry ClaimLocalAdmin: %v", err)
	}
}

func TestLocalAdminClaimReconcilesAfterRollbackFailure(t *testing.T) {
	directoryErr := errors.New("read users directory")
	rollbackErr := errors.New("delete local credential")
	store := &authTestStore{deleteErr: rollbackErr}
	users := newAuthTestUsers()
	users.isRegisteredErr = directoryErr
	service := newAuthTestService(t, store, users, User{})

	_, err := service.ClaimLocalAdmin(context.Background(), "admin@example.com", "correct horse battery staple", "")
	if !errors.Is(err, directoryErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("ClaimLocalAdmin error = %v, want directory and rollback errors", err)
	}
	if store.local == nil || !service.LocalAdminConfigured() {
		t.Fatal("rollback failure was not reconciled with the durable credential")
	}
}

func TestGoogleCannotBootstrapAdministrator(t *testing.T) {
	store := &authTestStore{oauth: OAuthConfig{GoogleClientID: "id", GoogleClientSecret: "secret"}}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{Email: "stranger@example.com", Sub: "google-user"})

	if _, err := service.LoginGoogle(context.Background(), "code"); err == nil {
		t.Fatal("uninvited Google user claimed an empty server")
	} else {
		var notInvited NotInvitedError
		if !errors.As(err, &notInvited) {
			t.Fatalf("LoginGoogle error = %v", err)
		}
	}
	if len(users.roles) != 0 {
		t.Fatalf("Google login created users: %#v", users.roles)
	}
}

func TestLegacyAdminMustAuthorizeLocalPasswordSetup(t *testing.T) {
	store := &authTestStore{oauth: OAuthConfig{GoogleClientID: "id", GoogleClientSecret: "secret"}}
	users := newAuthTestUsers()
	users.roles["admin@example.com"] = true
	service := newAuthTestService(t, store, users, User{})

	if _, err := service.ClaimLocalAdmin(context.Background(), "admin@example.com", "correct horse battery staple", ""); !errors.Is(err, ErrAdminClaimUnauthorized) {
		t.Fatalf("unauthorized legacy claim error = %v", err)
	}
	if _, err := service.ClaimLocalAdmin(context.Background(), "admin@example.com", "correct horse battery staple", "admin@example.com"); err != nil {
		t.Fatalf("authorized legacy claim: %v", err)
	}
}

func TestConfigureGoogleOAuthAtRuntime(t *testing.T) {
	store := &authTestStore{}
	service := newAuthTestService(t, store, newAuthTestUsers(), User{})
	if service.GoogleOAuthEnabled() {
		t.Fatal("Google OAuth started enabled")
	}
	if err := service.ConfigureGoogleOAuth(context.Background(), OAuthConfig{
		GoogleClientID: "client-id", GoogleClientSecret: "client-secret",
	}); err != nil {
		t.Fatalf("ConfigureGoogleOAuth: %v", err)
	}
	if !service.GoogleOAuthEnabled() || service.GoogleClientID() != "client-id" {
		t.Fatal("Google OAuth was not enabled at runtime")
	}
}

func TestLocalAdministratorCannotUseGoogleLogin(t *testing.T) {
	store := &authTestStore{oauth: OAuthConfig{GoogleClientID: "id", GoogleClientSecret: "secret"}}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{Email: "admin@example.com", Sub: "google-admin"})
	if _, err := service.ClaimLocalAdmin(context.Background(), "admin@example.com", "correct horse battery staple", ""); err != nil {
		t.Fatalf("ClaimLocalAdmin: %v", err)
	}
	if _, err := service.LoginGoogle(context.Background(), "code"); !errors.Is(err, ErrLocalAdminPasswordOnly) {
		t.Fatalf("local admin Google login error = %v", err)
	}
}

func TestClaimInvalidatesLegacyGoogleAdminSession(t *testing.T) {
	store := &authTestStore{}
	users := newAuthTestUsers()
	users.roles["admin@example.com"] = true
	service := newAuthTestService(t, store, users, User{})
	legacySession := service.codec.sign(User{Email: "admin@example.com", Sub: "google-subject"}, "")

	if _, err := service.ClaimLocalAdmin(
		context.Background(),
		"admin@example.com",
		"correct horse battery staple",
		"admin@example.com",
	); err != nil {
		t.Fatalf("ClaimLocalAdmin: %v", err)
	}
	if _, err := service.CurrentSession(context.Background(), legacySession); !errors.Is(err, ErrLocalAdminPasswordOnly) {
		t.Fatalf("legacy Google session error = %v, want %v", err, ErrLocalAdminPasswordOnly)
	}

	localSession := service.codec.sign(User{Email: "admin@example.com", Sub: "local-admin"}, "")
	if _, err := service.CurrentSession(context.Background(), localSession); err != nil {
		t.Fatalf("local admin session: %v", err)
	}
}
