package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var localAdminEmailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// LocalAdminAuthenticator owns the local credential and its claim/login
// invariants. Service delegates to it to preserve the public facade.
type LocalAdminAuthenticator struct {
	store       LocalAdminStore
	users       UserDirectory
	setupTokens *setupTokenGuard

	mu         sync.RWMutex
	credential *LocalAdminCredential
	dummyHash  string
	claimMu    sync.Mutex
}

func newLocalAdminAuthenticator(
	store LocalAdminStore,
	users UserDirectory,
	setupTokens *setupTokenGuard,
	credential *LocalAdminCredential,
) *LocalAdminAuthenticator {
	return &LocalAdminAuthenticator{
		store: store, users: users, setupTokens: setupTokens, credential: credential,
	}
}

func (a *LocalAdminAuthenticator) setDummyHash(hash string) {
	a.mu.Lock()
	a.dummyHash = hash
	a.mu.Unlock()
}

func (a *LocalAdminAuthenticator) claim(ctx context.Context, req ClaimRequest) (User, error) {
	email := normalizeEmail(req.Email)
	if !localAdminEmailPattern.MatchString(email) {
		return User{}, errors.New("valid admin email is required")
	}

	a.claimMu.Lock()
	defer a.claimMu.Unlock()
	a.mu.RLock()
	alreadyClaimed := a.credential != nil
	a.mu.RUnlock()
	if alreadyClaimed {
		return User{}, ErrLocalAdminAlreadyClaimed
	}

	if a.users == nil {
		return User{}, errors.New("users directory is not configured")
	}
	first, err := a.users.FirstAdmin(ctx)
	if err != nil {
		return User{}, err
	}
	// Two different things can authorise a claim. Once an administrator
	// exists, they vouch for it. On a genuinely first boot nobody can, and
	// the token printed to the server terminal is the only thing standing
	// between whoever loads the page first and ownership of the server.
	tokenGated := first == nil
	if tokenGated {
		if err := a.setupTokens.verify(ctx, req.SetupToken); err != nil {
			return User{}, err
		}
	} else {
		authorizedEmail := normalizeEmail(req.AuthorizedEmail)
		isAdmin, authErr := a.users.IsAdmin(ctx, authorizedEmail)
		if authErr != nil {
			return User{}, authErr
		}
		if !isAdmin || authorizedEmail != email || email != normalizeEmail(first.Email) {
			return User{}, ErrAdminClaimUnauthorized
		}
	}
	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		return User{}, err
	}

	credential := LocalAdminCredential{Email: email, PasswordHash: passwordHash}
	if err := a.store.CreateLocalAdmin(ctx, credential); err != nil {
		return User{}, err
	}

	registered, err := a.users.IsRegistered(ctx, email)
	if err != nil {
		return User{}, a.abortClaim(ctx, credential, err)
	}
	if !registered {
		if err := a.users.AddBootstrapAdmin(ctx, email); err != nil {
			return User{}, a.abortClaim(ctx, credential, err)
		}
	}
	a.setCredential(&credential)
	if tokenGated {
		// Only now is the claim irreversible, so only now may the token be
		// spent: every path above this line is one the operator can retry.
		// Failing to mark it used is deliberately not fatal - the credential
		// is already written, and the already-claimed check at the top of
		// this method rejects every later attempt regardless.
		_ = a.setupTokens.consume(ctx)
	}
	return localAdminUser(email), nil
}

func (a *LocalAdminAuthenticator) abortClaim(
	ctx context.Context,
	credential LocalAdminCredential,
	claimErr error,
) error {
	rollbackCtx := context.WithoutCancel(ctx)
	rollbackErr := a.store.DeleteLocalAdmin(rollbackCtx, credential)
	if rollbackErr == nil {
		return claimErr
	}

	// A failed compare-and-delete means the durable state is uncertain. Reload
	// it so this process cannot advertise an unclaimed state that a restart
	// would interpret as configured.
	persisted, reconcileErr := a.store.LocalAdmin(rollbackCtx)
	if reconcileErr == nil {
		a.setCredential(persisted)
	} else {
		a.setCredential(&credential)
	}

	errs := []error{
		claimErr,
		fmt.Errorf("roll back local admin credential: %w", rollbackErr),
	}
	if reconcileErr != nil {
		errs = append(errs, fmt.Errorf("reload local admin credential: %w", reconcileErr))
	}
	return errors.Join(errs...)
}

func (a *LocalAdminAuthenticator) setCredential(credential *LocalAdminCredential) {
	var copy *LocalAdminCredential
	if credential != nil {
		value := *credential
		copy = &value
	}
	a.mu.Lock()
	a.credential = copy
	a.mu.Unlock()
}

func (a *LocalAdminAuthenticator) login(email, password string) (User, error) {
	a.mu.RLock()
	credential := a.credential
	hash := a.dummyHash
	if credential != nil {
		copy := *credential
		credential = &copy
		hash = copy.PasswordHash
	}
	a.mu.RUnlock()

	passwordOK, err := VerifyPassword(hash, password)
	if err != nil {
		return User{}, ErrInvalidCredentials
	}
	emailOK := credential != nil && normalizeEmail(email) == credential.Email
	if !emailOK || !passwordOK {
		return User{}, ErrInvalidCredentials
	}
	return localAdminUser(credential.Email), nil
}

func (a *LocalAdminAuthenticator) configured() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.credential != nil
}

func (a *LocalAdminAuthenticator) isLocalAdmin(email string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.credential != nil && a.credential.Email == normalizeEmail(email)
}

func (a *LocalAdminAuthenticator) adminEmail() (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.credential == nil {
		return "", false
	}
	return a.credential.Email, true
}

func localAdminUser(email string) User {
	return User{Email: normalizeEmail(email), Sub: "local-admin"}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
