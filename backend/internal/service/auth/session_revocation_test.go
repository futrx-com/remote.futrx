package auth

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestSessionRevocationCutoff(t *testing.T) {
	const email = "user@example.com"
	r := newTestSessionRegistry()
	if err := r.RevokeSessionsBefore(context.Background(), email, time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		iat  int64
		want error
	}{
		{"before", 99, ErrSessionRevoked}, {"equal", 100, nil}, {"after", 101, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := r.IsActive(context.Background(), Session{Email: email, Iat: tc.iat}); !errors.Is(err, tc.want) {
				t.Fatalf("IsActive = %v, want %v", err, tc.want)
			}
		})
	}
	// Revocation takes precedence when a session is also superseded.
	if err := r.SetPreferences(context.Background(), email, SecurityPreferences{SingleSessionEnabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := r.IsActive(context.Background(), Session{Email: email, Iat: 99, SID: "stale"}); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("IsActive = %v, want revoked", err)
	}
	if err := r.IsActive(context.Background(), Session{Email: email, Iat: 100, SID: "stale"}); !errors.Is(err, ErrSessionSuperseded) {
		t.Fatalf("IsActive = %v, want superseded", err)
	}
}

func TestRevocationPreservesAccountStateAndIsMonotonic(t *testing.T) {
	const email = "user@example.com"
	store := newAuthTestSessionRegistryStore()
	original := SessionRegistryRecord{
		Preferences:     SecurityPreferences{SingleSessionEnabled: true, HistoryEnabled: true, RecoveryCodeAlertEnabled: true},
		ActiveSessionID: "active", History: SessionHistory{Entries: []SessionRecord{{SID: "history"}}}, Alert: &SecurityAlert{IP: "1.2.3.4"},
	}
	store.records[email] = original
	r := newSessionRegistry(store, 20)
	for _, cutoff := range []int64{100, 90, 100} {
		if err := r.RevokeSessionsBefore(context.Background(), " USER@example.com ", time.Unix(cutoff, 0)); err != nil {
			t.Fatal(err)
		}
	}
	want := original
	want.SessionsValidAfter = 100
	if got := store.records[email]; !reflect.DeepEqual(got, want) {
		t.Fatalf("record = %+v, want %+v", got, want)
	}
	if err := r.Revoke(context.Background(), email); err != nil {
		t.Fatal(err)
	}
	if got := store.records[email].SessionsValidAfter; got != 100 {
		t.Fatalf("logout cutoff = %d", got)
	}
	if err := r.SetPreferences(context.Background(), email, SecurityPreferences{}); err != nil {
		t.Fatal(err)
	}
	if got := store.records[email].SessionsValidAfter; got != 100 {
		t.Fatalf("preferences cutoff = %d", got)
	}
}

type revocationSaveStore struct {
	*authTestSessionRegistryStore
	saveErr error
	saves   int
}

func (s *revocationSaveStore) Save(ctx context.Context, email string, record SessionRegistryRecord) error {
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.authTestSessionRegistryStore.Save(ctx, email, record)
}

func TestRevocationOnlyRecordKeepsSessionIssuanceUntracked(t *testing.T) {
	service := newAuthTestService(t, &authTestStore{}, newAuthTestUsers(), User{})
	store := &revocationSaveStore{authTestSessionRegistryStore: newAuthTestSessionRegistryStore()}
	service.registry = newSessionRegistry(store, 20)
	const email = "user@example.com"
	if err := service.registry.RevokeSessionsBefore(context.Background(), email, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if prefs := store.records[email].Preferences; prefs != (SecurityPreferences{}) {
		t.Fatalf("preferences = %+v", prefs)
	}
	cookie, err := service.IssueSession(context.Background(), User{Email: email}, SignInMethodPassword, "", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.CurrentSession(context.Background(), cookie)
	if err != nil {
		t.Fatal(err)
	}
	if session.SID != "" || store.saves != 1 {
		t.Fatalf("untracked issuance: sid=%q saves=%d", session.SID, store.saves)
	}
}

func TestRevocationFailuresDoNotPublishCutoff(t *testing.T) {
	const email = "user@example.com"
	t.Run("read", func(t *testing.T) {
		r := newSessionRegistry(failingSessionRegistryReadStore{newAuthTestSessionRegistryStore()}, 20)
		if err := r.RevokeSessionsBefore(context.Background(), email, time.Unix(100, 0)); err == nil {
			t.Fatal("expected read failure")
		}
		if len(r.cache) != 0 {
			t.Fatal("failed load was cached")
		}
	})
	t.Run("save", func(t *testing.T) {
		store := &revocationSaveStore{authTestSessionRegistryStore: newAuthTestSessionRegistryStore(), saveErr: errors.New("save failed")}
		r := newSessionRegistry(store, 20)
		if err := r.RevokeSessionsBefore(context.Background(), email, time.Unix(100, 0)); !errors.Is(err, store.saveErr) {
			t.Fatalf("error = %v", err)
		}
		if err := r.IsActive(context.Background(), Session{Email: email, Iat: 99}); err != nil {
			t.Fatalf("failed save revoked session: %v", err)
		}
		if len(store.records) != 0 {
			t.Fatal("failed save persisted a record")
		}
	})
}

// The first read captures the old record then pauses. The second read lets a
// revocation finish before the first reader tries to publish its stale result.
type coldRevocationRaceStore struct {
	mu      sync.Mutex
	calls   int
	record  *SessionRegistryRecord
	started chan struct{}
	release chan struct{}
}

func (s *coldRevocationRaceStore) Get(context.Context, string) (*SessionRegistryRecord, error) {
	s.mu.Lock()
	s.calls++
	first := s.calls == 1
	var captured *SessionRegistryRecord
	if s.record != nil {
		copy := *s.record
		captured = &copy
	}
	s.mu.Unlock()
	if first {
		close(s.started)
		<-s.release
	}
	return captured, nil
}
func (s *coldRevocationRaceStore) Save(_ context.Context, _ string, record SessionRegistryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.record = &record
	return nil
}
func (s *coldRevocationRaceStore) Delete(context.Context, string) error { return nil }

func TestColdLoadCannotOverwriteRevocation(t *testing.T) {
	for _, initial := range []struct {
		name   string
		record *SessionRegistryRecord
	}{
		{name: "missing"}, {name: "existing", record: &SessionRegistryRecord{SessionsValidAfter: 50}},
	} {
		t.Run(initial.name, func(t *testing.T) {
			store := &coldRevocationRaceStore{record: initial.record, started: make(chan struct{}), release: make(chan struct{})}
			r := newSessionRegistry(store, 20)
			result := make(chan error, 1)
			go func() { result <- r.IsActive(context.Background(), Session{Email: "user@example.com", Iat: 99}) }()
			<-store.started
			if err := r.RevokeSessionsBefore(context.Background(), "user@example.com", time.Unix(100, 0)); err != nil {
				close(store.release)
				t.Fatal(err)
			}
			close(store.release)
			if err := <-result; !errors.Is(err, ErrSessionRevoked) {
				t.Fatalf("cold reader = %v, want revoked", err)
			}
			if err := r.IsActive(context.Background(), Session{Email: "user@example.com", Iat: 99}); !errors.Is(err, ErrSessionRevoked) {
				t.Fatalf("cached reader = %v, want revoked", err)
			}
		})
	}
}
