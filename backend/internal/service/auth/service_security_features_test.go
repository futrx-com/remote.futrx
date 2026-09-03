package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

const testAdminPassword = "correct horse battery staple"

func newTestServiceWithLocalAdmin(t *testing.T) (*Service, string) {
	t.Helper()
	store := &authTestStore{}
	users := newAuthTestUsers()
	service := newAuthTestService(t, store, users, User{})
	email := "admin@example.com"
	if _, err := service.ClaimLocalAdmin(context.Background(), email, testAdminPassword, ""); err != nil {
		t.Fatalf("ClaimLocalAdmin: %v", err)
	}
	return service, email
}

// TestAllSecurityFlagsOffLeavesLoginUnchanged is the "2FA off, single-session
// off, history off, alert off" baseline every other case in this file is
// compared against.
func TestAllSecurityFlagsOffLeavesLoginUnchanged(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)

	result, err := service.CompletePasswordLogin(context.Background(), email, testAdminPassword, "1.2.3.4", "ua-1")
	if err != nil {
		t.Fatalf("CompletePasswordLogin: %v", err)
	}
	if !result.Completed || result.CookieValue == "" {
		t.Fatalf("expected an immediately completed login, got %+v", result)
	}
	session, err := service.CurrentSession(context.Background(), result.CookieValue)
	if err != nil {
		t.Fatalf("CurrentSession: %v", err)
	}
	if session.SID != "" {
		t.Fatalf("SID = %q, want empty when no SecurityPreferences flag is on", session.SID)
	}

	// Logging in again from a different "device" does not invalidate the
	// first cookie, matching today's unconditional multi-device behavior.
	second, err := service.CompletePasswordLogin(context.Background(), email, testAdminPassword, "5.6.7.8", "ua-2")
	if err != nil {
		t.Fatalf("CompletePasswordLogin (2nd): %v", err)
	}
	if _, err := service.CurrentSession(context.Background(), result.CookieValue); err != nil {
		t.Fatalf("first cookie should still be valid: %v", err)
	}
	if _, err := service.CurrentSession(context.Background(), second.CookieValue); err != nil {
		t.Fatalf("second cookie should be valid: %v", err)
	}
}

// TestTwoFactorEnabledRequiresChallengeIndependentlyOfOtherFlags exercises
// the 2FA-only branch: password login returns a pending result, and the
// verify-equivalent call (CompleteTwoFactorChallenge) completes it.
func TestTwoFactorEnabledRequiresChallengeIndependentlyOfOtherFlags(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	enrollToken, _, _, err := service.twoFactor.BeginEnrollment(context.Background(), email)
	if err != nil {
		t.Fatalf("BeginEnrollment: %v", err)
	}
	pending, err := service.twoFactor.codec.verify(enrollToken)
	if err != nil {
		t.Fatalf("decode enrollment token: %v", err)
	}
	code := TOTPCode(pending.Secret, time.Now())
	if _, _, err := service.twoFactor.ConfirmEnrollment(context.Background(), email, enrollToken, code); err != nil {
		t.Fatalf("ConfirmEnrollment: %v", err)
	}

	result, err := service.CompletePasswordLogin(context.Background(), email, testAdminPassword, "", "")
	if err != nil {
		t.Fatalf("CompletePasswordLogin: %v", err)
	}
	if result.Completed || result.PendingToken == "" {
		t.Fatalf("expected a pending 2FA challenge, got %+v", result)
	}

	nextCode := TOTPCode(pending.Secret, time.Now())
	completed, err := service.CompleteTwoFactorChallenge(context.Background(), result.PendingToken, nextCode, "", "")
	if err != nil {
		t.Fatalf("CompleteTwoFactorChallenge: %v", err)
	}
	if !completed.Completed || completed.CookieValue == "" {
		t.Fatalf("expected the challenge to complete the login, got %+v", completed)
	}
	if _, err := service.CurrentSession(context.Background(), completed.CookieValue); err != nil {
		t.Fatalf("CurrentSession after 2FA challenge: %v", err)
	}
}

// TestSingleSessionSupersedesWithoutTwoFactor confirms single active session
// is independent of 2FA: a plain password login, with no 2FA challenge
// involved, still supersedes an earlier device once the flag is on.
func TestSingleSessionSupersedesWithoutTwoFactor(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	if err := service.registry.SetPreferences(context.Background(), email, SecurityPreferences{SingleSessionEnabled: true}); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}

	first, err := service.CompletePasswordLogin(context.Background(), email, testAdminPassword, "1.1.1.1", "ua-1")
	if err != nil {
		t.Fatalf("CompletePasswordLogin (1st): %v", err)
	}
	if !first.Completed {
		t.Fatalf("expected immediate completion with 2FA off, got %+v", first)
	}
	if _, err := service.CurrentSession(context.Background(), first.CookieValue); err != nil {
		t.Fatalf("first session should be valid immediately: %v", err)
	}

	second, err := service.CompletePasswordLogin(context.Background(), email, testAdminPassword, "2.2.2.2", "ua-2")
	if err != nil {
		t.Fatalf("CompletePasswordLogin (2nd): %v", err)
	}
	if _, err := service.CurrentSession(context.Background(), first.CookieValue); !errors.Is(err, ErrSessionSuperseded) {
		t.Fatalf("first session error = %v, want %v", err, ErrSessionSuperseded)
	}
	if _, err := service.CurrentSession(context.Background(), second.CookieValue); err != nil {
		t.Fatalf("second session should be valid: %v", err)
	}
}

// TestHistoryEnabledAloneRecordsSignIns confirms history works without
// single-session enforcement.
func TestHistoryEnabledAloneRecordsSignIns(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	if err := service.registry.SetPreferences(context.Background(), email, SecurityPreferences{HistoryEnabled: true}); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}

	if _, err := service.CompletePasswordLogin(context.Background(), email, testAdminPassword, "9.9.9.9", "test-agent"); err != nil {
		t.Fatalf("CompletePasswordLogin: %v", err)
	}
	history, err := service.registry.History(context.Background(), email)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history.Entries) != 1 || history.Entries[0].IP != "9.9.9.9" || history.Entries[0].Method != SignInMethodPassword {
		t.Fatalf("history = %+v, want one password sign-in from 9.9.9.9", history)
	}

	// Logging in again does not force single-session supersession, since
	// that flag is independently off.
	if _, err := service.CompletePasswordLogin(context.Background(), email, testAdminPassword, "8.8.8.8", "ua"); err != nil {
		t.Fatalf("CompletePasswordLogin (2nd): %v", err)
	}
	history, _ = service.registry.History(context.Background(), email)
	if len(history.Entries) != 2 {
		t.Fatalf("history = %+v, want two entries", history)
	}
}

// TestRecoveryCodeAlertRequiresTwoFactorAndFiresOnRecoveryCodeUse checks that
// the alert is populated on Status only when RecoveryCodeAlertEnabled is on
// and a login actually used a recovery code, and clears on AckAlert.
func TestRecoveryCodeAlertRequiresTwoFactorAndFiresOnRecoveryCodeUse(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	enrollToken, _, _, err := service.twoFactor.BeginEnrollment(context.Background(), email)
	if err != nil {
		t.Fatalf("BeginEnrollment: %v", err)
	}
	pending, err := service.twoFactor.codec.verify(enrollToken)
	if err != nil {
		t.Fatalf("decode enrollment token: %v", err)
	}
	code := TOTPCode(pending.Secret, time.Now())
	recoveryCodes, _, err := service.twoFactor.ConfirmEnrollment(context.Background(), email, enrollToken, code)
	if err != nil {
		t.Fatalf("ConfirmEnrollment: %v", err)
	}
	if err := service.registry.SetPreferences(context.Background(), email, SecurityPreferences{RecoveryCodeAlertEnabled: true}); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}

	result, err := service.CompletePasswordLogin(context.Background(), email, testAdminPassword, "", "")
	if err != nil {
		t.Fatalf("CompletePasswordLogin: %v", err)
	}
	if result.Completed {
		t.Fatal("expected a pending 2FA challenge")
	}
	completed, err := service.CompleteTwoFactorChallenge(context.Background(), result.PendingToken, recoveryCodes[0], "", "")
	if err != nil {
		t.Fatalf("CompleteTwoFactorChallenge with recovery code: %v", err)
	}
	if !completed.Completed {
		t.Fatalf("expected the recovery-code challenge to complete the login: %+v", completed)
	}

	status := service.Status(context.Background(), completed.CookieValue)
	if status.SecurityAlert == nil {
		t.Fatal("Status did not surface the recovery-code alert")
	}
	if status.SecurityAlert.Method != SignInMethodPasswordRecoveryCode {
		t.Fatalf("alert method = %q, want %q", status.SecurityAlert.Method, SignInMethodPasswordRecoveryCode)
	}

	if err := service.registry.AckAlert(context.Background(), email); err != nil {
		t.Fatalf("AckAlert: %v", err)
	}
	status = service.Status(context.Background(), completed.CookieValue)
	if status.SecurityAlert != nil {
		t.Fatalf("alert still present after AckAlert: %+v", status.SecurityAlert)
	}
}
