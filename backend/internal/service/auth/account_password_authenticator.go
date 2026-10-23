package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrAccountCredentialChanged reports that an invited account's password was
// issued or changed since the caller read it, so the caller's view is stale.
var ErrAccountCredentialChanged = errors.New("account password no longer matches")

// PasswordState is what the users list needs to know about an invited
// account's password: that it has one is implied by being in the map.
type PasswordState struct {
	ChangePending bool
}

// accountPasswordAuthenticator owns the password credentials of invited
// accounts. Every credential is loaded at startup, so reads on the login path
// never touch the disk; writes go to the store first and reach the cache only
// once they are durable.
type accountPasswordAuthenticator struct {
	store AccountCredentialStore
	now   func() time.Time

	// account serializes each email's read-modify-write, and callers hold it
	// across a wider workflow (verify, revoke, replace). mu only guards the
	// cache map.
	account keyedMutex

	mu    sync.RWMutex
	cache map[string]AccountCredential
}

func newAccountPasswordAuthenticator(ctx context.Context, store AccountCredentialStore) (*accountPasswordAuthenticator, error) {
	stored, err := store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load account credentials: %w", err)
	}
	cache := make(map[string]AccountCredential, len(stored))
	for email, credential := range stored {
		cache[normalizeEmail(email)] = credential
	}
	return &accountPasswordAuthenticator{store: store, now: time.Now, cache: cache}, nil
}

// lock returns the unlock func of email's account lock. issueLocked and
// replaceLocked require it held.
func (a *accountPasswordAuthenticator) lock(email string) func() {
	return a.account.lock(normalizeEmail(email))
}

func (a *accountPasswordAuthenticator) credential(email string) (AccountCredential, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	credential, ok := a.cache[normalizeEmail(email)]
	return credential, ok
}

func (a *accountPasswordAuthenticator) states() map[string]PasswordState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	states := make(map[string]PasswordState, len(a.cache))
	for email, credential := range a.cache {
		states[email] = PasswordState{ChangePending: credential.MustChangePassword}
	}
	return states
}

// issueLocked installs a temporary password: the account must change it
// before it can sign in, whatever credential it had before.
func (a *accountPasswordAuthenticator) issueLocked(ctx context.Context, email, hash string) error {
	email = normalizeEmail(email)
	current, _ := a.credential(email)
	return a.saveLocked(ctx, email, AccountCredential{
		PasswordHash:       hash,
		MustChangePassword: true,
		Version:            current.Version + 1,
	})
}

// replaceLocked installs a password its owner chose, but only if the stored
// credential is still at expectedVersion: a reset or another change in the
// meantime fails with ErrAccountCredentialChanged instead of being overwritten.
func (a *accountPasswordAuthenticator) replaceLocked(ctx context.Context, email string, expectedVersion int64, hash string) error {
	email = normalizeEmail(email)
	current, ok := a.credential(email)
	if !ok || current.Version != expectedVersion {
		return ErrAccountCredentialChanged
	}
	return a.saveLocked(ctx, email, AccountCredential{
		PasswordHash: hash,
		Version:      current.Version + 1,
	})
}

func (a *accountPasswordAuthenticator) saveLocked(ctx context.Context, email string, next AccountCredential) error {
	next.UpdatedAt = a.now().UTC()
	if err := a.store.Save(ctx, email, next); err != nil {
		return fmt.Errorf("save account credential: %w", err)
	}
	a.mu.Lock()
	a.cache[email] = next
	a.mu.Unlock()
	return nil
}
