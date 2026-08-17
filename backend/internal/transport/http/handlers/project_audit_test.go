package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
)

// TestProjectCreateIsAudited is the end-to-end check on the whole path the
// feature rests on: the auth middleware resolves the caller onto the request
// context, the project service records against it, the file store appends a
// JSONL line, and the admin read API returns it.
func TestProjectCreateIsAudited(t *testing.T) {
	mux, auth, auditLog := newAuditTestHandler(t)

	repo, err := fileproject.NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("project store: %v", err)
	}
	projects := serviceproject.New(
		repo,
		serviceproject.ContainerDependencies{},
		nil,
		nil,
		serviceproject.WithAudit(auditLog),
	)
	NewProjectHandler(projects, nil, auth, "remote.futrx.com").RegisterRoutes(mux)

	handler := httpmiddleware.NewAuth(auth).
		RequireLocalAdminSetup(func() bool { return true }).
		RequireProviderLogin(func() bool { return true }).
		Wrap(mux)

	session := auth.SignSession(serviceauth.User{Email: "admin@example.com", Sub: "local-admin"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(`{"name":"Audited Project"}`))
	createReq.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: session})
	createReq.Header.Set("X-Forwarded-For", "203.0.113.11")
	createReq.Header.Set("User-Agent", "integration-test/1.0")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create project = %d (%s)", createRec.Code, createRec.Body.String())
	}
	var created serviceproject.Meta
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode project: %v", err)
	}

	queryReq := httptest.NewRequest(http.MethodGet, "/api/admin/audit?action=project.create", nil)
	queryReq.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: session})
	queryRec := httptest.NewRecorder()
	handler.ServeHTTP(queryRec, queryReq)
	if queryRec.Code != http.StatusOK {
		t.Fatalf("audit query = %d (%s)", queryRec.Code, queryRec.Body.String())
	}

	var page serviceaudit.Page
	if err := json.Unmarshal(queryRec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode audit page: %v", err)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("audit entries = %d, want exactly the project create", len(page.Entries))
	}

	entry := page.Entries[0]
	if entry.Action != serviceaudit.ActionProjectCreate {
		t.Fatalf("action = %q", entry.Action)
	}
	if !entry.OK || entry.Error != "" {
		t.Fatalf("entry = %+v, want a success", entry)
	}
	if entry.Actor.Email != "admin@example.com" || !entry.Actor.IsAdmin {
		t.Fatalf("actor = %+v, want the session admin", entry.Actor)
	}
	if entry.Actor.Sub != "local-admin" {
		t.Fatalf("actor sub = %q, want the session subject", entry.Actor.Sub)
	}
	if entry.Target.Type != serviceaudit.TargetProject || entry.Target.ID != string(created.ID) {
		t.Fatalf("target = %+v, want the created project", entry.Target)
	}
	if entry.Target.Name != "Audited Project" {
		t.Fatalf("target name = %q", entry.Target.Name)
	}
	if entry.IP != "203.0.113.11" || entry.UserAgent != "integration-test/1.0" {
		t.Fatalf("request fields = %q/%q", entry.IP, entry.UserAgent)
	}
	if entry.Meta["slug"] != created.Slug {
		t.Fatalf("meta slug = %v, want %q", entry.Meta["slug"], created.Slug)
	}
	if entry.At.IsZero() {
		t.Fatal("entry has no timestamp")
	}
}

// TestProjectMemberRemovalIsAudited pins the action that used to discard the
// caller identity outright ("available for future audit logging").
func TestProjectMemberRemovalIsAudited(t *testing.T) {
	mux, auth, auditLog := newAuditTestHandler(t)

	repo, err := fileproject.NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("project store: %v", err)
	}
	access := &memoryProjectAccess{members: map[serviceproject.ID][]string{}}
	projects := serviceproject.New(
		repo,
		serviceproject.ContainerDependencies{},
		nil,
		access,
		serviceproject.WithAudit(auditLog),
	)
	NewProjectHandler(projects, nil, auth, "remote.futrx.com").RegisterRoutes(mux)

	handler := httpmiddleware.NewAuth(auth).
		RequireLocalAdminSetup(func() bool { return true }).
		RequireProviderLogin(func() bool { return true }).
		Wrap(mux)
	session := auth.SignSession(serviceauth.User{Email: "admin@example.com", Sub: "local-admin"})

	createReq := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(`{"name":"Shared"}`))
	createReq.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: session})
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create project = %d (%s)", createRec.Code, createRec.Body.String())
	}
	var created serviceproject.Meta
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode project: %v", err)
	}

	removeReq := httptest.NewRequest(
		http.MethodDelete,
		"/api/projects/"+string(created.ID)+"/access/admin%40example.com",
		nil,
	)
	removeReq.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: session})
	removeRec := httptest.NewRecorder()
	handler.ServeHTTP(removeRec, removeReq)
	if removeRec.Code != http.StatusOK {
		t.Fatalf("remove member = %d (%s)", removeRec.Code, removeRec.Body.String())
	}

	queryReq := httptest.NewRequest(
		http.MethodGet,
		"/api/admin/audit?action=project.member.remove&target="+string(created.ID),
		nil,
	)
	queryReq.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: session})
	queryRec := httptest.NewRecorder()
	handler.ServeHTTP(queryRec, queryReq)

	var page serviceaudit.Page
	if err := json.Unmarshal(queryRec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode audit page: %v", err)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("audit entries = %d, want the membership removal", len(page.Entries))
	}
	if got := page.Entries[0].Meta["member"]; got != "admin@example.com" {
		t.Fatalf("meta member = %v, want the removed address", got)
	}
	if page.Entries[0].Actor.Email != "admin@example.com" {
		t.Fatalf("actor = %+v, want the caller who removed the member", page.Entries[0].Actor)
	}
}

// memoryProjectAccess is an in-test membership store; the file-backed one is
// covered by its own package tests.
type memoryProjectAccess struct {
	members map[serviceproject.ID][]string
}

func (a *memoryProjectAccess) List(_ context.Context, id serviceproject.ID) ([]string, error) {
	return append([]string(nil), a.members[id]...), nil
}

func (a *memoryProjectAccess) Add(_ context.Context, id serviceproject.ID, email string) error {
	a.members[id] = append(a.members[id], email)
	return nil
}

func (a *memoryProjectAccess) Remove(_ context.Context, id serviceproject.ID, email string) error {
	kept := make([]string, 0, len(a.members[id]))
	for _, member := range a.members[id] {
		if member != email {
			kept = append(kept, member)
		}
	}
	a.members[id] = kept
	return nil
}

func (a *memoryProjectAccess) Set(_ context.Context, id serviceproject.ID, emails []string) error {
	a.members[id] = append([]string(nil), emails...)
	return nil
}

func (a *memoryProjectAccess) Has(_ context.Context, id serviceproject.ID, email string) (bool, error) {
	for _, member := range a.members[id] {
		if member == email {
			return true, nil
		}
	}
	return false, nil
}

func (a *memoryProjectAccess) DeleteAll(_ context.Context, id serviceproject.ID) error {
	delete(a.members, id)
	return nil
}
