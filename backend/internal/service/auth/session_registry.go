package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// sessionRegistry owns SecurityPreferences (the three independent
// single-session/history/alert toggles), the currently active session id
// per account, bounded sign-in history, and any pending recovery-code
// alert. Mutex + in-memory cache per the LocalAdminAuthenticator shape: an
// account's record - including a confirmed "nothing enabled" (nil) result -
// is cached lazily on first access, so accounts that never touch any of the
// three flags pay at most one file read per process lifetime, not one per
// request.
type sessionRegistry struct {
	store        SessionRegistryStore
	historyLimit int

	// account serializes each account's read-modify-write of its record so
	// concurrent sign-ins cannot clobber each other's history or active
	// session id; mu only guards the cache map itself.
	account keyedMutex

	mu    sync.RWMutex
	cache map[string]*SessionRegistryRecord
}

func newSessionRegistry(store SessionRegistryStore, historyLimit int) *sessionRegistry {
	return &sessionRegistry{
		store:        store,
		historyLimit: historyLimit,
		cache:        map[string]*SessionRegistryRecord{},
	}
}

func newSessionID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func (r *sessionRegistry) load(ctx context.Context, email string) (*SessionRegistryRecord, error) {
	email = normalizeEmail(email)
	r.mu.RLock()
	if record, ok := r.cache[email]; ok {
		r.mu.RUnlock()
		return record, nil
	}
	r.mu.RUnlock()

	record, err := r.store.Get(ctx, email)
	if err != nil {
		return nil, err
	}
	r.setCache(email, record)
	return record, nil
}

func (r *sessionRegistry) setCache(email string, record *SessionRegistryRecord) {
	r.mu.Lock()
	r.cache[normalizeEmail(email)] = record
	r.mu.Unlock()
}

// Preferences returns email's current SecurityPreferences, defaulting to
// all-off for an account that has never saved any.
func (r *sessionRegistry) Preferences(ctx context.Context, email string) (SecurityPreferences, error) {
	record, err := r.load(ctx, email)
	if err != nil {
		return SecurityPreferences{}, err
	}
	if record == nil {
		return SecurityPreferences{}, nil
	}
	return record.Preferences, nil
}

// SetPreferences overwrites email's SecurityPreferences. Callers do their
// own read-modify-write for partial updates.
func (r *sessionRegistry) SetPreferences(ctx context.Context, email string, prefs SecurityPreferences) error {
	email = normalizeEmail(email)
	defer r.account.lock(email)()
	record, err := r.load(ctx, email)
	if err != nil {
		return err
	}
	updated := SessionRegistryRecord{}
	if record != nil {
		updated = *record
	}
	updated.Preferences = prefs
	if err := r.store.Save(ctx, email, updated); err != nil {
		return err
	}
	r.setCache(email, &updated)
	return nil
}

// IssueForAccount records a new sign-in for email under method, applying
// whichever of the three independent SecurityPreferences flags are on:
// SingleSessionEnabled marks the new session id active and supersedes the
// previous one; HistoryEnabled appends a bounded (configured-limit,
// newest-first)
// history record; RecoveryCodeAlertEnabled sets an unacknowledged alert
// when method used a recovery code. It always returns a freshly generated
// session id, even when every flag is off, so callers can uniformly embed
// it in the signed session (harmless when nothing consults it).
func (r *sessionRegistry) IssueForAccount(ctx context.Context, email string, method SignInMethod, ip, userAgent string) (string, error) {
	email = normalizeEmail(email)
	sid, err := newSessionID()
	if err != nil {
		return "", err
	}

	defer r.account.lock(email)()
	record, err := r.load(ctx, email)
	if err != nil {
		return "", err
	}
	updated := SessionRegistryRecord{}
	if record != nil {
		updated = *record
	}

	now := time.Now().Unix()
	if updated.Preferences.SingleSessionEnabled {
		updated.ActiveSessionID = sid
	}
	if updated.Preferences.HistoryEnabled {
		entry := SessionRecord{SID: sid, Method: method, IP: ip, UserAgent: userAgent, IssuedAt: now}
		entries := append([]SessionRecord{entry}, updated.History.Entries...)
		if len(entries) > r.historyLimit {
			entries = entries[:r.historyLimit]
		}
		updated.History.Entries = entries
	}
	if updated.Preferences.RecoveryCodeAlertEnabled && method.usedRecoveryCode() {
		updated.Alert = &SecurityAlert{
			Method:     method,
			IP:         ip,
			UserAgent:  userAgent,
			OccurredAt: now,
		}
	}

	if err := r.store.Save(ctx, email, updated); err != nil {
		return "", err
	}
	r.setCache(email, &updated)
	return sid, nil
}

// IsActive reports whether sid is still the account's active session. Only
// meaningful (and only ever false for a previously-issued sid) once
// SingleSessionEnabled is on; before that, or for an account with no
// record, every issued sid is treated as active.
func (r *sessionRegistry) IsActive(ctx context.Context, email, sid string) bool {
	record, err := r.load(ctx, email)
	if err != nil || record == nil {
		return true
	}
	if !record.Preferences.SingleSessionEnabled {
		return true
	}
	return record.ActiveSessionID == sid
}

// Revoke replaces email's active session id with an unissued id (used on
// logout), a no-op if the account has no registry record. Keeping the revoked
// marker non-empty prevents legacy cookies without a SID from matching it.
func (r *sessionRegistry) Revoke(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	defer r.account.lock(email)()
	record, err := r.load(ctx, email)
	if err != nil {
		return err
	}
	if record == nil {
		return nil
	}
	updated := *record
	if updated.Preferences.SingleSessionEnabled {
		revokedSessionID, err := newSessionID()
		if err != nil {
			return err
		}
		updated.ActiveSessionID = revokedSessionID
	} else {
		updated.ActiveSessionID = ""
	}
	if err := r.store.Save(ctx, email, updated); err != nil {
		return err
	}
	r.setCache(email, &updated)
	return nil
}

// History returns email's bounded sign-in history, empty if the account has
// never enabled HistoryEnabled or has no registry record.
func (r *sessionRegistry) History(ctx context.Context, email string) (SessionHistory, error) {
	record, err := r.load(ctx, email)
	if err != nil {
		return SessionHistory{}, err
	}
	if record == nil {
		return SessionHistory{}, nil
	}
	return record.History, nil
}

// PendingAlert returns email's unacknowledged SecurityAlert, or nil if there
// is none.
func (r *sessionRegistry) PendingAlert(ctx context.Context, email string) (*SecurityAlert, error) {
	record, err := r.load(ctx, email)
	if err != nil || record == nil {
		return nil, err
	}
	return record.Alert, nil
}

// AckAlert clears email's pending alert, a no-op if there is none.
func (r *sessionRegistry) AckAlert(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	defer r.account.lock(email)()
	record, err := r.load(ctx, email)
	if err != nil {
		return err
	}
	if record == nil || record.Alert == nil {
		return nil
	}
	updated := *record
	updated.Alert = nil
	if err := r.store.Save(ctx, email, updated); err != nil {
		return err
	}
	r.setCache(email, &updated)
	return nil
}
