package auth

import (
	"context"
	"testing"
)

func newTestSessionRegistry() *sessionRegistry {
	return newSessionRegistry(newAuthTestSessionRegistryStore(), authTestOptions().SessionHistoryLimit)
}

func TestSingleSessionEnabledSupersedesEarlierSession(t *testing.T) {
	r := newTestSessionRegistry()
	email := "user@example.com"
	if err := r.SetPreferences(context.Background(), email, SecurityPreferences{SingleSessionEnabled: true}); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}

	sidA, err := r.IssueForAccount(context.Background(), email, SignInMethodPassword, "1.1.1.1", "ua-a")
	if err != nil {
		t.Fatalf("IssueForAccount (1st): %v", err)
	}
	if !r.IsActive(context.Background(), email, sidA) {
		t.Fatal("first session should be active immediately after issuance")
	}

	sidB, err := r.IssueForAccount(context.Background(), email, SignInMethodPassword, "2.2.2.2", "ua-b")
	if err != nil {
		t.Fatalf("IssueForAccount (2nd): %v", err)
	}
	if r.IsActive(context.Background(), email, sidA) {
		t.Fatal("first session should be superseded by the second")
	}
	if !r.IsActive(context.Background(), email, sidB) {
		t.Fatal("second session should be active")
	}
}

func TestSingleSessionDisabledKeepsBothSessionsActive(t *testing.T) {
	r := newTestSessionRegistry()
	email := "user@example.com"
	// SingleSessionEnabled left off (default).
	sidA, err := r.IssueForAccount(context.Background(), email, SignInMethodPassword, "", "")
	if err != nil {
		t.Fatalf("IssueForAccount (1st): %v", err)
	}
	sidB, err := r.IssueForAccount(context.Background(), email, SignInMethodPassword, "", "")
	if err != nil {
		t.Fatalf("IssueForAccount (2nd): %v", err)
	}
	if !r.IsActive(context.Background(), email, sidA) || !r.IsActive(context.Background(), email, sidB) {
		t.Fatal("both sessions should remain active when single-session is off")
	}
}

func TestRevokeInvalidatesActiveAndLegacyEmptySessions(t *testing.T) {
	r := newTestSessionRegistry()
	email := "user@example.com"
	if err := r.SetPreferences(context.Background(), email, SecurityPreferences{SingleSessionEnabled: true}); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}
	sid, err := r.IssueForAccount(context.Background(), email, SignInMethodPassword, "", "")
	if err != nil {
		t.Fatalf("IssueForAccount: %v", err)
	}
	if err := r.Revoke(context.Background(), email); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if r.IsActive(context.Background(), email, sid) {
		t.Fatal("session should be inactive after Revoke")
	}
	if r.IsActive(context.Background(), email, "") {
		t.Fatal("legacy empty-SID session should remain inactive after Revoke")
	}
}

func TestHistoryOnlyRecordedWhenEnabled(t *testing.T) {
	r := newTestSessionRegistry()
	email := "user@example.com"

	if _, err := r.IssueForAccount(context.Background(), email, SignInMethodPassword, "", ""); err != nil {
		t.Fatalf("IssueForAccount: %v", err)
	}
	history, err := r.History(context.Background(), email)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history.Entries) != 0 {
		t.Fatalf("history recorded with HistoryEnabled off: %+v", history)
	}

	if err := r.SetPreferences(context.Background(), email, SecurityPreferences{HistoryEnabled: true}); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}
	if _, err := r.IssueForAccount(context.Background(), email, SignInMethodPassword, "1.2.3.4", "ua"); err != nil {
		t.Fatalf("IssueForAccount: %v", err)
	}
	history, err = r.History(context.Background(), email)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history.Entries) != 1 || history.Entries[0].IP != "1.2.3.4" {
		t.Fatalf("history = %+v, want one entry with recorded IP", history)
	}
}

func TestHistoryUsesConfiguredLimit(t *testing.T) {
	r := newSessionRegistry(newAuthTestSessionRegistryStore(), 2)
	email := "user@example.com"
	if err := r.SetPreferences(context.Background(), email, SecurityPreferences{HistoryEnabled: true}); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}
	for _, ip := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
		if _, err := r.IssueForAccount(context.Background(), email, SignInMethodPassword, ip, ""); err != nil {
			t.Fatalf("IssueForAccount(%s): %v", ip, err)
		}
	}
	history, err := r.History(context.Background(), email)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history.Entries) != 2 {
		t.Fatalf("history length = %d, want configured limit 2", len(history.Entries))
	}
	if history.Entries[0].IP != "3.3.3.3" || history.Entries[1].IP != "2.2.2.2" {
		t.Fatalf("history = %+v, want two newest entries", history.Entries)
	}
}

func TestAlertOnlySetWhenEnabledAndRecoveryCodeUsed(t *testing.T) {
	r := newTestSessionRegistry()
	email := "user@example.com"

	// Recovery-code login with the alert flag off: no alert.
	if _, err := r.IssueForAccount(context.Background(), email, SignInMethodPasswordRecoveryCode, "", ""); err != nil {
		t.Fatalf("IssueForAccount: %v", err)
	}
	if alert, _ := r.PendingAlert(context.Background(), email); alert != nil {
		t.Fatalf("alert set with RecoveryCodeAlertEnabled off: %+v", alert)
	}

	if err := r.SetPreferences(context.Background(), email, SecurityPreferences{RecoveryCodeAlertEnabled: true}); err != nil {
		t.Fatalf("SetPreferences: %v", err)
	}

	// Normal TOTP login with the flag on: still no alert.
	if _, err := r.IssueForAccount(context.Background(), email, SignInMethodPasswordTOTP, "", ""); err != nil {
		t.Fatalf("IssueForAccount: %v", err)
	}
	if alert, _ := r.PendingAlert(context.Background(), email); alert != nil {
		t.Fatalf("alert set for a non-recovery-code login: %+v", alert)
	}

	// Recovery-code login with the flag on: alert set.
	if _, err := r.IssueForAccount(context.Background(), email, SignInMethodGoogleRecoveryCode, "9.9.9.9", "ua"); err != nil {
		t.Fatalf("IssueForAccount: %v", err)
	}
	alert, err := r.PendingAlert(context.Background(), email)
	if err != nil {
		t.Fatalf("PendingAlert: %v", err)
	}
	if alert == nil || alert.Method != SignInMethodGoogleRecoveryCode {
		t.Fatalf("alert = %+v, want a recovery-code alert", alert)
	}

	if err := r.AckAlert(context.Background(), email); err != nil {
		t.Fatalf("AckAlert: %v", err)
	}
	if alert, _ := r.PendingAlert(context.Background(), email); alert != nil {
		t.Fatalf("alert still present after AckAlert: %+v", alert)
	}
}
