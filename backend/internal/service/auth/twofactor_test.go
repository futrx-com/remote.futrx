package auth

import (
	"context"
	"encoding/base32"
	"errors"
	"sync"
	"testing"
	"time"
)

func newTestTwoFactorAuthenticator() *twoFactorAuthenticator {
	return newTestTwoFactorAuthenticatorWithPublisher(newNoopLifecyclePublisher())
}

func newTestTwoFactorAuthenticatorWithPublisher(publisher UpdateLifecyclePublisher) *twoFactorAuthenticator {
	options := authTestOptions()
	return newTwoFactorAuthenticator(
		newAuthTestTwoFactorStore(),
		"remote.futrx",
		[]byte("test-key"),
		options.EnrollmentTTL,
		options.RecoveryCodeCount,
		publisher,
	)
}

// recordedLifecycleEvent is one call into recordingLifecyclePublisher, kept
// generic enough (kind + the same fields the real publisher takes) to assert
// both the started/terminal sequence and the exact values a producer sent.
type recordedLifecycleEvent struct {
	kind      string // "started", "completed", or "failed"
	source    string
	operation string
	subject   string
	err       error
}

// recordingLifecyclePublisher is the auth package's own UpdateLifecyclePublisher
// test double, used to verify the 2FA lifecycle wrapping in this file. It is
// distinct from the publishers package's own observer fakes, which verify the
// publisher's dispatch mechanics rather than what 2FA sends into it.
type recordingLifecyclePublisher struct {
	mu     sync.Mutex
	events []recordedLifecycleEvent
}

func newRecordingLifecyclePublisher() *recordingLifecyclePublisher {
	return &recordingLifecyclePublisher{}
}

func (p *recordingLifecyclePublisher) PublishUpdateStarted(_ context.Context, source, operation, subject string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, recordedLifecycleEvent{kind: "started", source: source, operation: operation, subject: subject})
}

func (p *recordingLifecyclePublisher) PublishUpdateCompleted(_ context.Context, source, operation, subject string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, recordedLifecycleEvent{kind: "completed", source: source, operation: operation, subject: subject})
}

func (p *recordingLifecyclePublisher) PublishUpdateFailed(_ context.Context, source, operation, subject string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, recordedLifecycleEvent{kind: "failed", source: source, operation: operation, subject: subject, err: err})
}

func (p *recordingLifecyclePublisher) snapshot() []recordedLifecycleEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]recordedLifecycleEvent, len(p.events))
	copy(out, p.events)
	return out
}

func (p *recordingLifecyclePublisher) kinds() []string {
	events := p.snapshot()
	kinds := make([]string, len(events))
	for i, e := range events {
		kinds[i] = e.kind
	}
	return kinds
}

func enrollTestAccount(t *testing.T, a *twoFactorAuthenticator, email string) []string {
	t.Helper()
	token, secretBase32, uri, err := a.BeginEnrollment(context.Background(), email)
	if err != nil {
		t.Fatalf("BeginEnrollment: %v", err)
	}
	if token == "" || secretBase32 == "" || uri == "" {
		t.Fatal("BeginEnrollment returned an empty field")
	}
	secret, err := a.codec.verify(token)
	if err != nil {
		t.Fatalf("decode enrollment token: %v", err)
	}
	code := TOTPCode(secret.Secret, time.Now())
	codes, confirmedEmail, err := a.ConfirmEnrollment(context.Background(), email, token, code)
	if err != nil {
		t.Fatalf("ConfirmEnrollment: %v", err)
	}
	if confirmedEmail != normalizeEmail(email) {
		t.Fatalf("confirmedEmail = %q, want %q", confirmedEmail, email)
	}
	if len(codes) != a.recoveryCodeCount {
		t.Fatalf("len(recovery codes) = %d, want %d", len(codes), a.recoveryCodeCount)
	}
	return codes
}

func TestTwoFactorEnrollmentRoundTrip(t *testing.T) {
	a := newTestTwoFactorAuthenticator()
	email := "user@example.com"

	if a.Enabled(context.Background(), email) {
		t.Fatal("2FA reported enabled before enrollment")
	}

	enrollTestAccount(t, a, email)

	if !a.Enabled(context.Background(), email) {
		t.Fatal("2FA not enabled after ConfirmEnrollment")
	}
}

func TestEnrollmentUsesConfiguredRecoveryCodeCount(t *testing.T) {
	a := newTwoFactorAuthenticator(
		newAuthTestTwoFactorStore(),
		"remote.futrx",
		[]byte("test-key"),
		10*time.Minute,
		3,
		newNoopLifecyclePublisher(),
	)
	codes := enrollTestAccount(t, a, "user@example.com")
	if len(codes) != 3 {
		t.Fatalf("len(recovery codes) = %d, want configured count 3", len(codes))
	}
}

func TestBeginEnrollmentRejectsAlreadyEnrolledAccount(t *testing.T) {
	a := newTestTwoFactorAuthenticator()
	email := "user@example.com"
	enrollTestAccount(t, a, email)

	if _, _, _, err := a.BeginEnrollment(context.Background(), email); !errors.Is(err, ErrTwoFactorAlreadyEnabled) {
		t.Fatalf("BeginEnrollment on enrolled account error = %v, want %v", err, ErrTwoFactorAlreadyEnabled)
	}
}

func TestConfirmEnrollmentRejectsWrongCode(t *testing.T) {
	a := newTestTwoFactorAuthenticator()
	token, _, _, err := a.BeginEnrollment(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("BeginEnrollment: %v", err)
	}
	if _, _, err := a.ConfirmEnrollment(context.Background(), "user@example.com", token, "000000"); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("ConfirmEnrollment with wrong code error = %v, want %v", err, ErrInvalidTwoFactorCode)
	}
}

func TestVerifyChallengeAcceptsTOTPAndRecoveryCode(t *testing.T) {
	a := newTestTwoFactorAuthenticator()
	email := "user@example.com"
	codes := enrollTestAccount(t, a, email)

	record, err := a.load(context.Background(), email)
	if err != nil || record == nil {
		t.Fatalf("load: %v, %v", record, err)
	}
	totp := TOTPCode(record.Secret, time.Now())
	usedRecovery, err := a.VerifyChallenge(context.Background(), email, totp)
	if err != nil {
		t.Fatalf("VerifyChallenge with TOTP: %v", err)
	}
	if usedRecovery {
		t.Fatal("VerifyChallenge reported recovery-code use for a TOTP code")
	}

	usedRecovery, err = a.VerifyChallenge(context.Background(), email, codes[0])
	if err != nil {
		t.Fatalf("VerifyChallenge with recovery code: %v", err)
	}
	if !usedRecovery {
		t.Fatal("VerifyChallenge did not report recovery-code use")
	}

	// The consumed recovery code cannot be reused.
	if _, err := a.VerifyChallenge(context.Background(), email, codes[0]); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("reused recovery code error = %v, want %v", err, ErrInvalidTwoFactorCode)
	}
}

func TestDisableRequiresProofOfPossession(t *testing.T) {
	a := newTestTwoFactorAuthenticator()
	email := "user@example.com"
	enrollTestAccount(t, a, email)

	if err := a.Disable(context.Background(), email, "000000"); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("Disable with wrong code error = %v, want %v", err, ErrInvalidTwoFactorCode)
	}
	if !a.Enabled(context.Background(), email) {
		t.Fatal("Disable with a wrong code disabled 2FA anyway")
	}

	record, _ := a.load(context.Background(), email)
	code := TOTPCode(record.Secret, time.Now())
	if err := a.Disable(context.Background(), email, code); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if a.Enabled(context.Background(), email) {
		t.Fatal("2FA still enabled after Disable")
	}
}

func TestRegenerateRecoveryCodesReplacesTheSet(t *testing.T) {
	a := newTestTwoFactorAuthenticator()
	email := "user@example.com"
	oldCodes := enrollTestAccount(t, a, email)

	record, _ := a.load(context.Background(), email)
	totp := TOTPCode(record.Secret, time.Now())
	newCodes, err := a.RegenerateRecoveryCodes(context.Background(), email, totp)
	if err != nil {
		t.Fatalf("RegenerateRecoveryCodes: %v", err)
	}
	if len(newCodes) != a.recoveryCodeCount {
		t.Fatalf("len(newCodes) = %d, want %d", len(newCodes), a.recoveryCodeCount)
	}

	if _, err := a.VerifyChallenge(context.Background(), email, oldCodes[0]); err == nil {
		t.Fatal("old recovery code still worked after regeneration")
	}
	if _, err := a.VerifyChallenge(context.Background(), email, newCodes[0]); err != nil {
		t.Fatalf("new recovery code did not work: %v", err)
	}
}

// testTwoFactorCache reads a's cache directly (same package) so
// characterization tests can tell "not yet cached" from "cached with the
// pre-mutation value" without going through load, which would silently fall
// back to the store on a cache miss and mask the very bug being pinned.
func testTwoFactorCache(a *twoFactorAuthenticator, email string) (record *TwoFactorRecord, cached bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	record, cached = a.cache[normalizeEmail(email)]
	return record, cached
}

func TestConfirmEnrollmentStoreFailureDoesNotUpdateCache(t *testing.T) {
	a := newTestTwoFactorAuthenticator()
	store := a.store.(*authTestTwoFactorStore)
	email := "user@example.com"

	token, _, _, err := a.BeginEnrollment(context.Background(), email)
	if err != nil {
		t.Fatalf("BeginEnrollment: %v", err)
	}
	pending, err := a.codec.verify(token)
	if err != nil {
		t.Fatalf("decode enrollment token: %v", err)
	}
	code := TOTPCode(pending.Secret, time.Now())

	store.saveErr = errors.New("store save failed")
	if _, _, err := a.ConfirmEnrollment(context.Background(), email, token, code); err == nil {
		t.Fatal("ConfirmEnrollment succeeded despite a store save failure")
	}
	// BeginEnrollment already lazily cached the "not enrolled" (nil) result;
	// the assertion is that the cache still reflects that absence, not that
	// nothing is cached at all.
	if cached, ok := testTwoFactorCache(a, email); ok && cached != nil {
		t.Fatal("cache held an enrolled record despite a store save failure")
	}
	if a.Enabled(context.Background(), email) {
		t.Fatal("2FA reported enabled despite a store save failure")
	}
}

func TestDisableStoreFailureLeavesCachedEnrollmentEnabled(t *testing.T) {
	a := newTestTwoFactorAuthenticator()
	store := a.store.(*authTestTwoFactorStore)
	email := "user@example.com"
	enrollTestAccount(t, a, email)

	record, err := a.load(context.Background(), email)
	if err != nil || record == nil {
		t.Fatalf("load: %v, %v", record, err)
	}
	code := TOTPCode(record.Secret, time.Now())

	store.deleteErr = errors.New("store delete failed")
	if err := a.Disable(context.Background(), email, code); err == nil {
		t.Fatal("Disable succeeded despite a store delete failure")
	}
	if !a.Enabled(context.Background(), email) {
		t.Fatal("cached enrollment was cleared despite a store delete failure")
	}
}

func TestInvalidProofDoesNotWriteOrDeleteState(t *testing.T) {
	a := newTestTwoFactorAuthenticator()
	store := a.store.(*authTestTwoFactorStore)
	email := "user@example.com"
	enrollTestAccount(t, a, email)
	// enrollTestAccount's ConfirmEnrollment already made one legitimate Save
	// call; the assertion is that the invalid-proof paths below make no
	// further store calls, not that the store was never touched.
	saveCalls, deleteCalls := store.saveCalls, store.deleteCalls

	if _, err := a.VerifyChallenge(context.Background(), email, "000000"); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("VerifyChallenge with invalid code error = %v, want %v", err, ErrInvalidTwoFactorCode)
	}
	if err := a.Disable(context.Background(), email, "000000"); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("Disable with invalid code error = %v, want %v", err, ErrInvalidTwoFactorCode)
	}
	if _, err := a.RegenerateRecoveryCodes(context.Background(), email, "000000"); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("RegenerateRecoveryCodes with invalid code error = %v, want %v", err, ErrInvalidTwoFactorCode)
	}
	if store.saveCalls != saveCalls || store.deleteCalls != deleteCalls {
		t.Fatalf(
			"invalid-proof paths touched the store: saveCalls=%d (was %d) deleteCalls=%d (was %d)",
			store.saveCalls, saveCalls, store.deleteCalls, deleteCalls,
		)
	}
}

func TestInvalidEnrollmentDoesNotWriteState(t *testing.T) {
	a := newTestTwoFactorAuthenticator()
	store := a.store.(*authTestTwoFactorStore)
	email := "user@example.com"

	if _, _, err := a.ConfirmEnrollment(context.Background(), email, "not-a-valid-token", "000000"); !errors.Is(err, ErrInvalidEnrollmentToken) {
		t.Fatalf("ConfirmEnrollment with invalid token error = %v, want %v", err, ErrInvalidEnrollmentToken)
	}
	token, _, _, err := a.BeginEnrollment(context.Background(), email)
	if err != nil {
		t.Fatalf("BeginEnrollment: %v", err)
	}
	if _, _, err := a.ConfirmEnrollment(context.Background(), "other@example.com", token, "000000"); !errors.Is(err, ErrEnrollmentTokenMismatch) {
		t.Fatalf("ConfirmEnrollment with mismatched email error = %v, want %v", err, ErrEnrollmentTokenMismatch)
	}
	if store.saveCalls != 0 || store.deleteCalls != 0 {
		t.Fatalf("invalid-enrollment paths touched the store: saveCalls=%d deleteCalls=%d", store.saveCalls, store.deleteCalls)
	}
}

func TestVerifyChallengeUpdatesStoreBeforeCacheBecomesObservable(t *testing.T) {
	a := newTestTwoFactorAuthenticator()
	store := a.store.(*authTestTwoFactorStore)
	email := "user@example.com"
	enrollTestAccount(t, a, email)

	record, err := a.load(context.Background(), email)
	if err != nil || record == nil {
		t.Fatalf("load: %v, %v", record, err)
	}
	totp := TOTPCode(record.Secret, time.Now())

	var observedDuringSave *TwoFactorRecord
	store.beforeSave = func(_ string, _ TwoFactorRecord) {
		if cached, ok := testTwoFactorCache(a, email); ok && cached != nil {
			c := *cached
			observedDuringSave = &c
		}
	}
	if _, err := a.VerifyChallenge(context.Background(), email, totp); err != nil {
		t.Fatalf("VerifyChallenge: %v", err)
	}
	if observedDuringSave == nil {
		t.Fatal("expected a cached record to be present while the store save was in flight")
	}
	if observedDuringSave.LastUsedTOTPCounter != record.LastUsedTOTPCounter {
		t.Fatalf(
			"cache already reflected the new counter while the store save was in flight: got %d, want unchanged %d",
			observedDuringSave.LastUsedTOTPCounter, record.LastUsedTOTPCounter,
		)
	}
}

func TestDisableDeletesStoreBeforeCacheBecomesObservable(t *testing.T) {
	a := newTestTwoFactorAuthenticator()
	store := a.store.(*authTestTwoFactorStore)
	email := "user@example.com"
	enrollTestAccount(t, a, email)

	record, err := a.load(context.Background(), email)
	if err != nil || record == nil {
		t.Fatalf("load: %v, %v", record, err)
	}
	code := TOTPCode(record.Secret, time.Now())

	sawEnabledDuringDelete := false
	store.beforeDelete = func(_ string) {
		if cached, ok := testTwoFactorCache(a, email); ok && cached != nil {
			sawEnabledDuringDelete = true
		}
	}
	if err := a.Disable(context.Background(), email, code); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if !sawEnabledDuringDelete {
		t.Fatal("expected the cache to still show the account enabled while the store delete was in flight")
	}
	if a.Enabled(context.Background(), email) {
		t.Fatal("cache still reports enabled after a successful Disable")
	}
}

func TestConfirmEnrollmentPublishesStartedThenCompletedOnSuccess(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	email := "user@example.com"

	enrollTestAccount(t, a, email)

	got := publisher.kinds()
	want := []string{"started", "completed"}
	if len(got) != len(want) {
		t.Fatalf("event kinds = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event kinds = %v, want %v", got, want)
		}
	}
	events := publisher.snapshot()
	for _, e := range events {
		if e.source != twoFactorLifecycleSource {
			t.Fatalf("source = %q, want %q", e.source, twoFactorLifecycleSource)
		}
		if e.operation != twoFactorLifecycleConfirmEnrollment {
			t.Fatalf("operation = %q, want %q", e.operation, twoFactorLifecycleConfirmEnrollment)
		}
		if e.subject != normalizeEmail(email) {
			t.Fatalf("subject = %q, want %q", e.subject, normalizeEmail(email))
		}
	}
}

func TestConfirmEnrollmentPublishesStartedThenFailedOnInvalidCode(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	email := "user@example.com"

	token, _, _, err := a.BeginEnrollment(context.Background(), email)
	if err != nil {
		t.Fatalf("BeginEnrollment: %v", err)
	}
	// BeginEnrollment's own Enabled/load check does not durably mutate state
	// and must emit nothing; only the failing ConfirmEnrollment below should.
	if len(publisher.snapshot()) != 0 {
		t.Fatalf("BeginEnrollment published events: %v", publisher.snapshot())
	}

	if _, _, err := a.ConfirmEnrollment(context.Background(), email, token, "000000"); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("ConfirmEnrollment error = %v, want %v", err, ErrInvalidTwoFactorCode)
	}

	events := publisher.snapshot()
	if len(events) != 2 || events[0].kind != "started" || events[1].kind != "failed" {
		t.Fatalf("events = %+v, want [started, failed]", events)
	}
	if !errors.Is(events[1].err, ErrInvalidTwoFactorCode) {
		t.Fatalf("failed event err = %v, want %v", events[1].err, ErrInvalidTwoFactorCode)
	}
	if events[1].subject != normalizeEmail(email) {
		t.Fatalf("failed event subject = %q, want %q", events[1].subject, normalizeEmail(email))
	}
}

func TestVerifyChallengeTOTPPublishesStartedThenCompleted(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	email := "user@example.com"
	enrollTestAccount(t, a, email)
	publisher.events = nil // isolate this challenge from enrollment's own events

	record, err := a.load(context.Background(), email)
	if err != nil || record == nil {
		t.Fatalf("load: %v, %v", record, err)
	}
	totp := TOTPCode(record.Secret, time.Now())
	if _, err := a.VerifyChallenge(context.Background(), email, totp); err != nil {
		t.Fatalf("VerifyChallenge: %v", err)
	}

	got := publisher.kinds()
	if len(got) != 2 || got[0] != "started" || got[1] != "completed" {
		t.Fatalf("event kinds = %v, want [started, completed]", got)
	}
	for _, e := range publisher.snapshot() {
		if e.operation != twoFactorLifecycleVerifyChallenge {
			t.Fatalf("operation = %q, want %q", e.operation, twoFactorLifecycleVerifyChallenge)
		}
	}

	// The replay-counter persistence this wrapping must not disturb: a second
	// use of the same code is rejected, and it still publishes started+failed.
	if _, err := a.VerifyChallenge(context.Background(), email, totp); !errors.Is(err, ErrTwoFactorCodeReused) {
		t.Fatalf("replayed TOTP error = %v, want %v", err, ErrTwoFactorCodeReused)
	}
	got = publisher.kinds()
	if len(got) != 4 || got[2] != "started" || got[3] != "failed" {
		t.Fatalf("event kinds after replay = %v, want [started, completed, started, failed]", got)
	}
}

func TestVerifyChallengeRecoveryCodePublishesStartedThenCompleted(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	email := "user@example.com"
	codes := enrollTestAccount(t, a, email)
	publisher.events = nil

	usedRecovery, err := a.VerifyChallenge(context.Background(), email, codes[0])
	if err != nil {
		t.Fatalf("VerifyChallenge with recovery code: %v", err)
	}
	if !usedRecovery {
		t.Fatal("expected recovery-code use to be reported")
	}
	got := publisher.kinds()
	if len(got) != 2 || got[0] != "started" || got[1] != "completed" {
		t.Fatalf("event kinds = %v, want [started, completed]", got)
	}

	// One-code consumption this wrapping must not disturb: reusing the same
	// recovery code is rejected, publishing started+failed.
	if _, err := a.VerifyChallenge(context.Background(), email, codes[0]); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("reused recovery code error = %v, want %v", err, ErrInvalidTwoFactorCode)
	}
	got = publisher.kinds()
	if len(got) != 4 || got[2] != "started" || got[3] != "failed" {
		t.Fatalf("event kinds after reuse = %v, want [started, completed, started, failed]", got)
	}
}

func TestVerifyChallengeInvalidCodePublishesStartedThenFailed(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	email := "user@example.com"
	enrollTestAccount(t, a, email)
	publisher.events = nil

	if _, err := a.VerifyChallenge(context.Background(), email, "000000"); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("VerifyChallenge error = %v, want %v", err, ErrInvalidTwoFactorCode)
	}
	got := publisher.kinds()
	if len(got) != 2 || got[0] != "started" || got[1] != "failed" {
		t.Fatalf("event kinds = %v, want [started, failed]", got)
	}
}

func TestDisablePublishesStartedThenCompletedOnSuccess(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	email := "user@example.com"
	enrollTestAccount(t, a, email)
	publisher.events = nil

	record, err := a.load(context.Background(), email)
	if err != nil || record == nil {
		t.Fatalf("load: %v, %v", record, err)
	}
	code := TOTPCode(record.Secret, time.Now())
	if err := a.Disable(context.Background(), email, code); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	got := publisher.kinds()
	if len(got) != 2 || got[0] != "started" || got[1] != "completed" {
		t.Fatalf("event kinds = %v, want [started, completed]", got)
	}
	for _, e := range publisher.snapshot() {
		if e.operation != twoFactorLifecycleDisable {
			t.Fatalf("operation = %q, want %q", e.operation, twoFactorLifecycleDisable)
		}
	}
}

func TestDisablePublishesStartedThenFailedOnInvalidCode(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	email := "user@example.com"
	enrollTestAccount(t, a, email)
	publisher.events = nil

	if err := a.Disable(context.Background(), email, "000000"); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("Disable error = %v, want %v", err, ErrInvalidTwoFactorCode)
	}
	got := publisher.kinds()
	if len(got) != 2 || got[0] != "started" || got[1] != "failed" {
		t.Fatalf("event kinds = %v, want [started, failed]", got)
	}
}

func TestRegenerateRecoveryCodesPublishesStartedThenCompletedOnSuccess(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	email := "user@example.com"
	enrollTestAccount(t, a, email)
	publisher.events = nil

	record, err := a.load(context.Background(), email)
	if err != nil || record == nil {
		t.Fatalf("load: %v, %v", record, err)
	}
	totp := TOTPCode(record.Secret, time.Now())
	if _, err := a.RegenerateRecoveryCodes(context.Background(), email, totp); err != nil {
		t.Fatalf("RegenerateRecoveryCodes: %v", err)
	}

	got := publisher.kinds()
	if len(got) != 2 || got[0] != "started" || got[1] != "completed" {
		t.Fatalf("event kinds = %v, want [started, completed]", got)
	}
	for _, e := range publisher.snapshot() {
		if e.operation != twoFactorLifecycleRegenerateRecoveryCodes {
			t.Fatalf("operation = %q, want %q", e.operation, twoFactorLifecycleRegenerateRecoveryCodes)
		}
	}
}

func TestRegenerateRecoveryCodesPublishesStartedThenFailedOnInvalidCode(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	email := "user@example.com"
	enrollTestAccount(t, a, email)
	publisher.events = nil

	if _, err := a.RegenerateRecoveryCodes(context.Background(), email, "000000"); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("RegenerateRecoveryCodes error = %v, want %v", err, ErrInvalidTwoFactorCode)
	}
	got := publisher.kinds()
	if len(got) != 2 || got[0] != "started" || got[1] != "failed" {
		t.Fatalf("event kinds = %v, want [started, failed]", got)
	}
}

func TestStoreFailurePublishesTheIdenticalOriginalError(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	store := a.store.(*authTestTwoFactorStore)
	email := "user@example.com"
	enrollTestAccount(t, a, email)
	publisher.events = nil

	record, err := a.load(context.Background(), email)
	if err != nil || record == nil {
		t.Fatalf("load: %v, %v", record, err)
	}
	totp := TOTPCode(record.Secret, time.Now())

	saveErr := errors.New("store save failed")
	store.saveErr = saveErr
	_, gotErr := a.VerifyChallenge(context.Background(), email, totp)
	if !errors.Is(gotErr, saveErr) {
		t.Fatalf("VerifyChallenge error = %v, want %v", gotErr, saveErr)
	}

	events := publisher.snapshot()
	if len(events) != 2 || events[1].kind != "failed" {
		t.Fatalf("events = %+v, want a terminal failed event", events)
	}
	if !errors.Is(events[1].err, saveErr) {
		t.Fatalf("published failed err = %v, want the identical %v", events[1].err, saveErr)
	}
}

func TestLifecycleEventSubjectsAreNormalizedEmails(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	email := "User@Example.COM"

	enrollTestAccount(t, a, email)

	for _, e := range publisher.snapshot() {
		if e.subject != normalizeEmail(email) {
			t.Fatalf("subject = %q, want normalized %q", e.subject, normalizeEmail(email))
		}
		if e.subject == email {
			t.Fatal("subject was not normalized")
		}
	}
}

func TestLifecycleEventsCarryNoSecretOrProofMaterial(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	email := "user@example.com"
	codes := enrollTestAccount(t, a, email)

	record, err := a.load(context.Background(), email)
	if err != nil || record == nil {
		t.Fatalf("load: %v, %v", record, err)
	}
	totp := TOTPCode(record.Secret, time.Now())
	if _, err := a.VerifyChallenge(context.Background(), email, totp); err != nil {
		t.Fatalf("VerifyChallenge: %v", err)
	}
	if _, err := a.RegenerateRecoveryCodes(context.Background(), email, totp); err != nil {
		t.Fatalf("RegenerateRecoveryCodes: %v", err)
	}

	// UpdateEvent (the type these calls ultimately construct) only ever
	// carries Source, Operation, and Subject - this test pins that no
	// producer call site is ever given the secret, a code, or a recovery
	// code as an extra argument by checking every recorded subject against
	// the values that must never leak.
	forbidden := []string{
		base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(record.Secret),
		totp,
		codes[0],
	}
	for _, e := range publisher.snapshot() {
		for _, f := range forbidden {
			if e.subject == f {
				t.Fatalf("event subject leaked secret/proof material: %q", e.subject)
			}
		}
	}
}

func TestBeginEnrollmentAndReadOnlyMethodsEmitNothing(t *testing.T) {
	publisher := newRecordingLifecyclePublisher()
	a := newTestTwoFactorAuthenticatorWithPublisher(publisher)
	email := "user@example.com"

	if _, _, _, err := a.BeginEnrollment(context.Background(), email); err != nil {
		t.Fatalf("BeginEnrollment: %v", err)
	}
	a.Enabled(context.Background(), email)
	a.RecoveryCodesRemaining(context.Background(), email)

	if events := publisher.snapshot(); len(events) != 0 {
		t.Fatalf("read-only paths published events: %+v", events)
	}
}

// completionObserver calls back into the authenticator from
// PublishUpdateCompleted to prove the terminal callback runs after the
// per-account lock is released and after the settled state is visible - not
// under the 2FA lock, or this would deadlock, and not before the mutation, or
// Enabled would read stale state.
type completionObserver struct {
	a                  *twoFactorAuthenticator
	email              string
	wantCodesRemaining int
	sawEnabled         bool
	sawCodes           int
}

func (o *completionObserver) PublishUpdateStarted(context.Context, string, string, string) {}
func (o *completionObserver) PublishUpdateCompleted(ctx context.Context, _, _, _ string) {
	o.sawEnabled = o.a.Enabled(ctx, o.email)
	o.sawCodes = o.a.RecoveryCodesRemaining(ctx, o.email)
}
func (o *completionObserver) PublishUpdateFailed(context.Context, string, string, string, error) {}

func TestCompletionCallbackMayQuerySettledStateWithoutDeadlock(t *testing.T) {
	a := newTestTwoFactorAuthenticatorWithPublisher(newNoopLifecyclePublisher())
	email := "user@example.com"
	codes := enrollTestAccount(t, a, email)

	observer := &completionObserver{a: a, email: email, wantCodesRemaining: len(codes)}
	a.publisher = observer

	done := make(chan struct{})
	go func() {
		record, err := a.load(context.Background(), email)
		if err != nil || record == nil {
			t.Errorf("load: %v, %v", record, err)
			close(done)
			return
		}
		totp := TOTPCode(record.Secret, time.Now())
		if _, err := a.VerifyChallenge(context.Background(), email, totp); err != nil {
			t.Errorf("VerifyChallenge: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("completion callback querying settled state deadlocked")
	}

	if !observer.sawEnabled {
		t.Fatal("completion callback did not see the account as enabled")
	}
	if observer.sawCodes != observer.wantCodesRemaining {
		t.Fatalf("completion callback saw %d recovery codes remaining, want %d", observer.sawCodes, observer.wantCodesRemaining)
	}
}
