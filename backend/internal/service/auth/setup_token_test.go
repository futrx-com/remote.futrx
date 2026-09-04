package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

// setupTokenTestStore is the minimal SetupTokenStore the guard needs, with a
// hook for the read failure that must make claims fail closed.
type setupTokenTestStore struct {
	record  *SetupTokenRecord
	readErr error
}

func (s *setupTokenTestStore) SetupToken(context.Context) (*SetupTokenRecord, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	return s.record, nil
}

func (s *setupTokenTestStore) SaveSetupToken(_ context.Context, record SetupTokenRecord) error {
	s.record = &record
	return nil
}

// frozenClock lets the expiry tests move time without sleeping.
type frozenClock struct{ at time.Time }

func (c *frozenClock) now() time.Time          { return c.at }
func (c *frozenClock) advance(d time.Duration) { c.at = c.at.Add(d) }

func newGuardForTest(store *setupTokenTestStore, ttl time.Duration) (*setupTokenGuard, *frozenClock) {
	clock := &frozenClock{at: time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)}
	return newSetupTokenGuard(store, ttl, clock.now), clock
}

func TestSetupTokenIssueStoresOnlyTheHash(t *testing.T) {
	store := &setupTokenTestStore{}
	guard, _ := newGuardForTest(store, time.Hour)

	token, err := guard.issue(context.Background())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if token == "" {
		t.Fatal("Issue returned an empty token")
	}
	if store.record == nil {
		t.Fatal("Issue did not persist a record")
	}
	if store.record.Hash == token {
		t.Fatal("the plaintext token was persisted; only its hash may be stored")
	}
	if store.record.Used {
		t.Fatal("a freshly issued token must not be marked used")
	}
}

func TestSetupTokenVerifyAcceptsTheIssuedToken(t *testing.T) {
	store := &setupTokenTestStore{}
	guard, _ := newGuardForTest(store, time.Hour)

	token, err := guard.issue(context.Background())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if err := guard.verify(context.Background(), token); err != nil {
		t.Fatalf("Verify of the issued token = %v, want nil", err)
	}
}

func TestSetupTokenVerifyRejectsMissingAndWrongTokens(t *testing.T) {
	store := &setupTokenTestStore{}
	guard, _ := newGuardForTest(store, time.Hour)
	if _, err := guard.issue(context.Background()); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	for name, presented := range map[string]string{
		"empty": "",
		"wrong": "not-the-issued-token",
	} {
		t.Run(name, func(t *testing.T) {
			if err := guard.verify(context.Background(), presented); !errors.Is(err, ErrSetupTokenRequired) {
				t.Fatalf("Verify(%q) = %v, want ErrSetupTokenRequired", presented, err)
			}
		})
	}
}

// No token issued at all is the state a claimed server sits in, and it must
// reject every presented value rather than waving one through.
func TestSetupTokenVerifyRejectsWhenNoTokenIssued(t *testing.T) {
	guard, _ := newGuardForTest(&setupTokenTestStore{}, time.Hour)

	if err := guard.verify(context.Background(), "anything"); !errors.Is(err, ErrSetupTokenRequired) {
		t.Fatalf("Verify with no issued token = %v, want ErrSetupTokenRequired", err)
	}
}

func TestSetupTokenVerifyRejectsExpiredToken(t *testing.T) {
	store := &setupTokenTestStore{}
	guard, clock := newGuardForTest(store, 30*time.Minute)

	token, err := guard.issue(context.Background())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	clock.advance(31 * time.Minute)

	if err := guard.verify(context.Background(), token); !errors.Is(err, ErrSetupTokenRequired) {
		t.Fatalf("Verify of expired token = %v, want ErrSetupTokenRequired", err)
	}
}

func TestSetupTokenVerifyRejectsConsumedToken(t *testing.T) {
	store := &setupTokenTestStore{}
	guard, _ := newGuardForTest(store, time.Hour)

	token, err := guard.issue(context.Background())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if err := guard.verify(context.Background(), token); err != nil {
		t.Fatalf("first Verify: %v", err)
	}
	if err := guard.consume(context.Background()); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	if err := guard.verify(context.Background(), token); !errors.Is(err, ErrSetupTokenRequired) {
		t.Fatalf("Verify after Consume = %v, want ErrSetupTokenRequired", err)
	}
}

// Verify must leave the token spendable: the claim it authorises can still
// fail on a weak password, and burning the token there would strand the
// operator with no way back in.
func TestSetupTokenVerifyDoesNotConsume(t *testing.T) {
	store := &setupTokenTestStore{}
	guard, _ := newGuardForTest(store, time.Hour)

	token, err := guard.issue(context.Background())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	for i := range 3 {
		if err := guard.verify(context.Background(), token); err != nil {
			t.Fatalf("Verify %d = %v, want nil", i+1, err)
		}
	}
	if store.record.Used {
		t.Fatal("Verify marked the token used; only Consume may do that")
	}
}

// A rotation must kill whatever was printed before, so the operator can never
// have two live tokens in two terminals.
func TestSetupTokenIssueRotatesPreviousToken(t *testing.T) {
	store := &setupTokenTestStore{}
	guard, _ := newGuardForTest(store, time.Hour)

	first, err := guard.issue(context.Background())
	if err != nil {
		t.Fatalf("first Issue: %v", err)
	}
	second, err := guard.issue(context.Background())
	if err != nil {
		t.Fatalf("second Issue: %v", err)
	}
	if first == second {
		t.Fatal("Issue returned the same token twice")
	}
	if err := guard.verify(context.Background(), first); !errors.Is(err, ErrSetupTokenRequired) {
		t.Fatalf("Verify of rotated-away token = %v, want ErrSetupTokenRequired", err)
	}
	if err := guard.verify(context.Background(), second); err != nil {
		t.Fatalf("Verify of current token = %v, want nil", err)
	}
}

// An unreadable token file must not read as "no gate in place".
func TestSetupTokenVerifyFailsClosedOnStoreError(t *testing.T) {
	store := &setupTokenTestStore{readErr: errors.New("permission denied")}
	guard, _ := newGuardForTest(store, time.Hour)

	err := guard.verify(context.Background(), "anything")
	if err == nil {
		t.Fatal("Verify succeeded despite an unreadable token store")
	}
	if !errors.Is(err, ErrSetupTokenUnavailable) {
		t.Fatalf("Verify store-error = %v, want ErrSetupTokenUnavailable", err)
	}
}
