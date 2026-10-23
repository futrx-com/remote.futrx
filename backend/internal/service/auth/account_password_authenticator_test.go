package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func newTestAccountAuthenticator(t *testing.T) (*accountPasswordAuthenticator, *authTestAccountCredentialStore) {
	t.Helper()
	store := newAuthTestAccountCredentialStore()
	a, err := newAccountPasswordAuthenticator(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	return a, store
}

func TestAccountPasswordAuthenticatorLoadsEverythingAtStartup(t *testing.T) {
	store := newAuthTestAccountCredentialStore()
	store.records["a@example.com"] = AccountCredential{PasswordHash: "ha", MustChangePassword: true, Version: 4}
	store.records["B@Example.com"] = AccountCredential{PasswordHash: "hb", Version: 2}
	a, err := newAccountPasswordAuthenticator(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := a.credential(" A@example.com"); !ok || got.PasswordHash != "ha" || got.Version != 4 {
		t.Fatalf("credential = %+v, %v", got, ok)
	}
	if got, ok := a.credential("b@example.com"); !ok || got.PasswordHash != "hb" {
		t.Fatalf("keys are not normalized: %+v, %v", got, ok)
	}
	if _, ok := a.credential("none@example.com"); ok {
		t.Fatal("unknown account has a credential")
	}
	// Reads are served from memory: dropping the backing store changes nothing.
	store.records = nil
	if _, ok := a.credential("a@example.com"); !ok {
		t.Fatal("read went back to the store")
	}
	states := a.states()
	if len(states) != 2 || !states["a@example.com"].ChangePending || states["b@example.com"].ChangePending {
		t.Fatalf("states = %+v", states)
	}
}

func TestAccountPasswordAuthenticatorStartupFailsWhenStoreUnreadable(t *testing.T) {
	store := newAuthTestAccountCredentialStore()
	store.listErr = errors.New("disk on fire")
	if _, err := newAccountPasswordAuthenticator(context.Background(), store); !errors.Is(err, store.listErr) {
		t.Fatalf("err = %v, want the list failure", err)
	}
}

func TestIssueLockedIncrementsVersionAndForcesAChange(t *testing.T) {
	a, store := newTestAccountAuthenticator(t)
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	ctx := context.Background()

	unlock := a.lock("A@example.com")
	defer unlock()
	if err := a.issueLocked(ctx, "A@example.com", "hash1"); err != nil {
		t.Fatal(err)
	}
	first, _ := a.credential("a@example.com")
	if first.Version != 1 || !first.MustChangePassword || first.PasswordHash != "hash1" || !first.UpdatedAt.Equal(now) {
		t.Fatalf("first = %+v", first)
	}
	// A password its owner chose is replaced by a new temporary one.
	if err := a.replaceLocked(ctx, "a@example.com", 1, "hash2"); err != nil {
		t.Fatal(err)
	}
	if err := a.issueLocked(ctx, "a@example.com", "hash3"); err != nil {
		t.Fatal(err)
	}
	third, _ := a.credential("a@example.com")
	if third.Version != 3 || !third.MustChangePassword || third.PasswordHash != "hash3" {
		t.Fatalf("third = %+v", third)
	}
	if stored := store.records["a@example.com"]; stored != third {
		t.Fatalf("store %+v != cache %+v", stored, third)
	}
}

func TestReplaceLockedIsACompareAndSwapOnVersion(t *testing.T) {
	a, store := newTestAccountAuthenticator(t)
	ctx := context.Background()
	defer a.lock("a@example.com")()
	if err := a.replaceLocked(ctx, "a@example.com", 0, "h"); !errors.Is(err, ErrAccountCredentialChanged) {
		t.Fatalf("replace of a missing credential = %v", err)
	}
	if err := a.issueLocked(ctx, "a@example.com", "temp"); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []int64{0, 2} {
		if err := a.replaceLocked(ctx, "a@example.com", stale, "h"); !errors.Is(err, ErrAccountCredentialChanged) {
			t.Fatalf("replace at version %d = %v", stale, err)
		}
	}
	if got, _ := a.credential("a@example.com"); got.PasswordHash != "temp" || got.Version != 1 {
		t.Fatalf("stale replace modified the credential: %+v", got)
	}
	if err := a.replaceLocked(ctx, "a@example.com", 1, "chosen"); err != nil {
		t.Fatal(err)
	}
	got, _ := a.credential("a@example.com")
	if got.PasswordHash != "chosen" || got.Version != 2 || got.MustChangePassword {
		t.Fatalf("after replace = %+v", got)
	}
	if store.records["a@example.com"] != got {
		t.Fatal("store and cache disagree")
	}
}

func TestStoreFailureLeavesCacheAndStoreUntouched(t *testing.T) {
	a, store := newTestAccountAuthenticator(t)
	ctx := context.Background()
	defer a.lock("a@example.com")()
	if err := a.issueLocked(ctx, "a@example.com", "temp"); err != nil {
		t.Fatal(err)
	}
	before, _ := a.credential("a@example.com")
	store.saveErr = errors.New("disk full")
	if err := a.issueLocked(ctx, "a@example.com", "x"); !errors.Is(err, store.saveErr) {
		t.Fatalf("issue = %v", err)
	}
	if err := a.replaceLocked(ctx, "a@example.com", before.Version, "y"); !errors.Is(err, store.saveErr) {
		t.Fatalf("replace = %v", err)
	}
	if after, _ := a.credential("a@example.com"); after != before {
		t.Fatalf("cache advanced past a failed save: %+v", after)
	}
	if store.records["a@example.com"] != before {
		t.Fatal("store changed")
	}
	// The failure is not sticky.
	store.saveErr = nil
	if err := a.replaceLocked(ctx, "a@example.com", before.Version, "y"); err != nil {
		t.Fatalf("retry = %v", err)
	}
}

func TestCacheIsUpdatedOnlyAfterTheStoreAccepts(t *testing.T) {
	a, store := newTestAccountAuthenticator(t)
	defer a.lock("a@example.com")()
	store.beforeSave = func(email string, _ AccountCredential) {
		if _, ok := a.credential(email); ok {
			t.Error("cache already holds the credential while the store is still writing")
		}
	}
	if err := a.issueLocked(context.Background(), "a@example.com", "h"); err != nil {
		t.Fatal(err)
	}
}

func TestPerAccountLockSerializesWritersButNotOtherAccounts(t *testing.T) {
	a, _ := newTestAccountAuthenticator(t)
	ctx := context.Background()
	const writers = 30
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := a.lock("a@example.com")
			defer unlock()
			if err := a.issueLocked(ctx, "a@example.com", "h"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got, _ := a.credential("a@example.com"); got.Version != writers {
		t.Fatalf("version = %d, want %d (lost update)", got.Version, writers)
	}

	unlock := a.lock("a@example.com")
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer a.lock("other@example.com")()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("another account was blocked by a@example.com's lock")
	}
	unlock()
}
