package httpmiddleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	"github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type workspaceChats struct{ project chat.ProjectID }

func (c workspaceChats) Get(context.Context, chat.ID) (chat.Meta, error) {
	return chat.Meta{ProjectID: c.project}, nil
}

type workspacePolicy struct {
	checks []rbac.Check
	err    error
}

func (p *workspacePolicy) Require(_ context.Context, c rbac.Check) error {
	p.checks = append(p.checks, c)
	return p.err
}
func TestWorkspaceDenialsPrecedeFilesystemOperationsOperations(t *testing.T) {
	for _, tc := range []struct{ path, capability string }{
		{"/api/chats/abcd/files", "files"}, {"/api/chats/abcd/files/search", "files"},
		{"/api/chats/abcd/files/download", "files"}, {"/api/chats/abcd/files/download-folder", "files"},
		{"/api/chats/abcd/media-open", "files"}, {"/api/chats/abcd/history/checkout", "git"},
		{"/api/chats/abcd/history/repos", "git"}, {"/api/chats/abcd/history/diff", "git"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			policy := &workspacePolicy{err: rbac.ErrDenied}
			called := false
			handler := (Workspace{Authorizer: policy, Chats: workspaceChats{project: "beef"}}).Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest("GET", tc.path, nil))
			if rec.Code != 403 || called || len(policy.checks) != 1 || policy.checks[0].Scope != rbac.ProjectScope("beef") || policy.checks[0].Permission != rbac.Key("workspace."+tc.capability+".use") {
				t.Fatalf("denial: status=%d called=%v checks=%v", rec.Code, called, policy.checks)
			}
			policy.err = nil
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", tc.path, nil))
			if !called {
				t.Fatal("allowed request not passed")
			}
		})
	}
}

func TestWorkspaceMissingPolicyFailsClosed(t *testing.T) {
	handler := (Workspace{Chats: workspaceChats{project: "beef"}}).Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("missing policy allowed terminal") }))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/chats/abcd/files", nil))
	if rec.Code != 403 {
		t.Fatal(rec.Code)
	}
}
