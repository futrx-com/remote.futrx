package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filesessions"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filetwofactor"
)

type uploadIdentities struct{ serviceauth.UserDirectory }

func (uploadIdentities) IsAdmin(context.Context, string) (bool, error)      { return true, nil }
func (uploadIdentities) IsRegistered(context.Context, string) (bool, error) { return true, nil }

type uploadChats struct{}

func (uploadChats) Get(context.Context, servicechat.ID) (servicechat.Meta, error) {
	return servicechat.Meta{ProjectID: "beef"}, nil
}

type uploadPolicy struct {
	denied bool
	check  rbac.Check
}

func (p *uploadPolicy) Require(ctx context.Context, c rbac.Check) error {
	p.check = c
	if _, ok := rbac.ActorFromContext(ctx); !ok {
		return rbac.ErrActorRequired
	}
	if p.denied {
		return rbac.ErrDenied
	}
	return nil
}

func TestUploadRequiresFileCapabilityEvenForAuthenticatedAdministrator(t *testing.T) {
	dir := t.TempDir()
	factors, err := filetwofactor.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := filesessions.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := serviceauth.New(context.Background(), fileauth.New(dir), uploadIdentities{}, func(string, string, string) serviceauth.OAuthProvider { return nil }, "https://remote.test", []byte("upload-permission-test-key"), factors, sessions, serviceauth.Options{PendingLoginTTL: time.Minute, EnrollmentTTL: time.Minute, RecoveryCodeCount: 5, SessionHistoryLimit: 10, SetupTokenTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueSession(context.Background(), serviceauth.User{Email: "admin@example.com"}, serviceauth.SignInMethodPassword, "", "")
	if err != nil {
		t.Fatal(err)
	}
	policy := &uploadPolicy{denied: true}
	gate := newAccessGate(auth, nil, uploadChats{})
	gate.authorizer = policy
	ctx := rbac.ContextWithActor(context.Background(), rbac.UserActor("admin@example.com"))
	req := httptest.NewRequest("POST", "https://remote.test/api/uploads", nil).WithContext(ctx)
	req.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: token})
	allowed, err := gate.CanUploadToChat(ctx, req, "abcd")
	if err != nil || allowed {
		t.Fatalf("denied upload: %v %v", allowed, err)
	}
	if policy.check.Permission != "workspace.files.use" || policy.check.Scope != rbac.ProjectScope("beef") {
		t.Fatal(policy.check)
	}
	policy.denied = false
	if allowed, err = gate.CanUploadToChat(ctx, req, "abcd"); err != nil || !allowed {
		t.Fatalf("allowed upload: %v %v", allowed, err)
	}
}
