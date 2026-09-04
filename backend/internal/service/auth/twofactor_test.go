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
