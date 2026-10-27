package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestTwoFactorAuthenticator() *twoFactorAuthenticator {
	options := authTestOptions()
	return newTwoFactorAuthenticator(
		newAuthTestTwoFactorStore(),
		"remote.futrx",
		[]byte("test-key"),
		options.EnrollmentTTL,
		options.RecoveryCodeCount,
	)
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
