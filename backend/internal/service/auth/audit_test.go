package auth

import (
	"context"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/audit"
)

type loginAuditRecorder struct{ entries []audit.Entry }

func (r *loginAuditRecorder) Record(_ context.Context, entry audit.Entry) {
	r.entries = append(r.entries, entry)
}

func TestAuditLoginWaitsForSecondFactor(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	secret, _ := enrollServiceAccount(t, service, email)
	recorder := &loginAuditRecorder{}
	WithAudit(recorder)(service)
	ctx := context.Background()

	login, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "203.0.113.1", "test")
	if err != nil || login.Completed || login.PendingToken == "" {
		t.Fatalf("expected pending second factor: %+v, %v", login, err)
	}
	if len(recorder.entries) != 0 {
		t.Fatalf("incomplete login recorded as successful: %+v", recorder.entries)
	}
	if _, err := service.CompleteTwoFactorChallenge(ctx, login.PendingToken, "invalid", "203.0.113.1", "test"); err == nil {
		t.Fatal("invalid second factor accepted")
	}
	if len(recorder.entries) != 1 || recorder.entries[0].Action != audit.ActionAuthLoginFailure || recorder.entries[0].Actor.Email != email {
		t.Fatalf("missing failed second-factor audit: %+v", recorder.entries)
	}
	completed, err := service.CompleteTwoFactorChallenge(ctx, login.PendingToken, TOTPCode(secret, time.Now()), "203.0.113.1", "test")
	if err != nil || !completed.Completed {
		t.Fatalf("complete login: %+v, %v", completed, err)
	}
	if len(recorder.entries) != 2 {
		t.Fatalf("audit count = %d, want failed attempt and completed login", len(recorder.entries))
	}
	entry := recorder.entries[1]
	if entry.Action != audit.ActionAuthLoginSuccess || !entry.OK || entry.Actor.Email != email || entry.Meta["method"] != string(SignInMethodPasswordTOTP) {
		t.Fatalf("completed login audit: %+v", entry)
	}
}

func TestAuditPasswordLoginRecordsOneResult(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	recorder := &loginAuditRecorder{}
	WithAudit(recorder)(service)
	ctx := context.Background()
	if _, err := service.CompletePasswordLogin(ctx, email, "wrong", "", ""); err == nil {
		t.Fatal("invalid password accepted")
	}
	if _, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "", ""); err != nil {
		t.Fatal(err)
	}
	if len(recorder.entries) != 2 || recorder.entries[0].Action != audit.ActionAuthLoginFailure || recorder.entries[1].Action != audit.ActionAuthLoginSuccess {
		t.Fatalf("expected one audit entry per login: %+v", recorder.entries)
	}
}
