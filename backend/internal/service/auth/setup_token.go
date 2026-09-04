package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const (
	// setupTokenBytes is the entropy behind the printed token. At 32 bytes the
	// token is not guessable, so the rate limit in front of the claim endpoint
	// is defence in depth rather than the thing standing between an attacker
	// and the credential.
	setupTokenBytes = 32
)

var (
	// ErrSetupTokenRequired is returned for every rejected token - missing,
	// wrong, expired, or already used. They deliberately share one error so a
	// caller cannot probe the difference.
	ErrSetupTokenRequired = errors.New("a valid setup token is required; run: remote setup-token")
	// ErrSetupTokenUnavailable means the token state could not be read or
	// written. Claims fail closed on it rather than falling through.
	ErrSetupTokenUnavailable = errors.New("setup token state is unavailable")
)

// setupTokenGuard owns the first-boot setup token: it issues one, checks a
// presented token against the stored hash, and marks it used. All state lives
// in the store rather than in memory, so a token issued by the CLI in a
// separate process is honoured by the already-running server without a
// restart.
type setupTokenGuard struct {
	store SetupTokenStore
	ttl   time.Duration
	now   func() time.Time
}

func newSetupTokenGuard(store SetupTokenStore, ttl time.Duration, now func() time.Time) *setupTokenGuard {
	if now == nil {
		now = time.Now
	}
	return &setupTokenGuard{store: store, ttl: ttl, now: now}
}

// SetupTokenIssuer owns the operator workflow around the token guard: issue a
// token only while no local credential or directory administrator can
// authorize the claim. Keeping this use case separate lets the setup-token
// CLI depend on setup state alone rather than constructing OAuth, sessions,
// and two-factor authentication.
type SetupTokenIssuer struct {
	tokens               *setupTokenGuard
	admins               SetupTokenAdminDirectory
	localAdminConfigured func() bool
}

// NewSetupTokenIssuer builds the standalone setup-token use case. It loads
// local-admin presence once because the CLI performs one command and exits;
// the server uses newSetupTokenIssuer with its live in-memory auth state.
func NewSetupTokenIssuer(
	ctx context.Context,
	store SetupTokenIssuerStore,
	admins SetupTokenAdminDirectory,
	ttl time.Duration,
) (*SetupTokenIssuer, error) {
	if store == nil {
		return nil, errors.New("setup token store is required")
	}
	if admins == nil {
		return nil, errors.New("admin directory is required")
	}
	if err := validateSetupTokenTTL(ttl); err != nil {
		return nil, err
	}
	credential, err := store.LocalAdmin(ctx)
	if err != nil {
		return nil, err
	}
	return newSetupTokenIssuer(
		newSetupTokenGuard(store, ttl, time.Now),
		admins,
		func() bool { return credential != nil },
	), nil
}

func newSetupTokenIssuer(
	tokens *setupTokenGuard,
	admins SetupTokenAdminDirectory,
	localAdminConfigured func() bool,
) *SetupTokenIssuer {
	return &SetupTokenIssuer{
		tokens:               tokens,
		admins:               admins,
		localAdminConfigured: localAdminConfigured,
	}
}

// EnsureSetupToken rotates the setup token while a first-boot claim is gated
// on it, and returns an empty token once another authority exists.
func (i *SetupTokenIssuer) EnsureSetupToken(ctx context.Context) (string, error) {
	if i.LocalAdminConfigured() || i.admins == nil {
		return "", nil
	}
	first, err := i.admins.FirstAdmin(ctx)
	if err != nil {
		return "", err
	}
	if first != nil {
		return "", nil
	}
	return i.tokens.issue(ctx)
}

// SetupTokenTTL is how long a freshly issued setup token remains valid.
func (i *SetupTokenIssuer) SetupTokenTTL() time.Duration {
	return i.tokens.ttlValue()
}

// LocalAdminConfigured reports why a command found setup no longer pending.
func (i *SetupTokenIssuer) LocalAdminConfigured() bool {
	return i.localAdminConfigured != nil && i.localAdminConfigured()
}

func validateSetupTokenTTL(ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("setup token TTL must be positive")
	}
	return nil
}

// ttlValue is how long a token this guard issues stays valid.
func (g *setupTokenGuard) ttlValue() time.Duration { return g.ttl }

// issue generates a token, persists only its hash, and returns the plaintext.
// That return value is the single moment the token exists anywhere outside the
// terminal it gets printed to; it is never stored and never sent over HTTP.
func (g *setupTokenGuard) issue(ctx context.Context) (string, error) {
	raw := make([]byte, setupTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate setup token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	record := SetupTokenRecord{
		Hash:      hashSetupToken(token),
		ExpiresAt: g.now().Add(g.ttl),
	}
	if err := g.store.SaveSetupToken(ctx, record); err != nil {
		return "", fmt.Errorf("%w: %w", ErrSetupTokenUnavailable, err)
	}
	return token, nil
}

// Verify accepts presented only when it matches the stored hash and is neither
// expired nor already used. It does not mutate anything: the token is spent by
// Consume, and only once the claim it authorised has actually succeeded.
func (g *setupTokenGuard) verify(ctx context.Context, presented string) error {
	if presented == "" {
		return ErrSetupTokenRequired
	}
	record, err := g.store.SetupToken(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSetupTokenUnavailable, err)
	}
	if record == nil || record.Used || g.now().After(record.ExpiresAt) {
		return ErrSetupTokenRequired
	}
	expected := []byte(record.Hash)
	actual := []byte(hashSetupToken(presented))
	if subtle.ConstantTimeCompare(expected, actual) != 1 {
		return ErrSetupTokenRequired
	}
	return nil
}

// Consume marks the stored token used, which is what makes a token work
// exactly once even while it is still unexpired.
func (g *setupTokenGuard) consume(ctx context.Context) error {
	record, err := g.store.SetupToken(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSetupTokenUnavailable, err)
	}
	if record == nil {
		return nil
	}
	record.Used = true
	if err := g.store.SaveSetupToken(ctx, *record); err != nil {
		return fmt.Errorf("%w: %w", ErrSetupTokenUnavailable, err)
	}
	return nil
}

// hashSetupToken reduces a token to what gets persisted. A plain digest is the
// right tool here, unlike for passwords: the token carries a full 32 bytes of
// entropy, so there is no dictionary to slow an attacker down with.
func hashSetupToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
