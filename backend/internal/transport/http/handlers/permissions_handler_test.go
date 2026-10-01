package httphandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
)

func permissionRoutes(t *testing.T) (membershipRoutesFixture, *servicechat.Service) {
	t.Helper()
	f := newMembershipRoutesFixture(t)
	chats, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = chats.Close() })
	service := servicechat.New(chats, nil, nil, nil, servicechat.WithAuthorizer(f.permissions))
	mux := http.NewServeMux()
	NewPermissionsHandler(f.permissions).RegisterRoutes(mux)
	NewProjectHandler(f.projects, nil, f.auth, sharesPublicHostname, nil).RegisterRoutes(mux)
	NewChatHandler(service, servicechat.NewAccessService(service, f.projects), f.auth, nil, nil, nil).RegisterRoutes(mux)
	f.handler = httpmiddleware.NewAuth(f.auth).Wrap(mux)
	return f, service
}

func TestPermissionManagementRoutesRequireAuthorityAndStrictInput(t *testing.T) {
	f, _ := permissionRoutes(t)
	for _, path := range []string{"/api/permissions", "/api/permissions/roles"} {
		method := http.MethodGet
		if path != "/api/permissions" {
			method = http.MethodPost
		}
		if r := f.do(t, "member@example.com", method, path, `{}`); r.Code != 403 {
			t.Fatalf("%s: %d %s", path, r.Code, r.Body)
		}
		if r := f.do(t, "", method, path, `{}`); r.Code != 401 {
			t.Fatalf("anonymous %s: %d", path, r.Code)
		}
	}
	for _, body := range []string{`{"UserEmail":"member@example.com","Permission":"projects.project.create","Effect":"deny","Scope":{"kind":"platform"},"actor":"admin@example.com"}`, `{} {}`, `null`} {
		if r := f.do(t, "admin@example.com", "PUT", "/api/permissions/assignments", body); r.Code != 400 {
			t.Fatalf("invalid body: %d %s", r.Code, r.Body)
		}
	}
	if r := f.do(t, "admin@example.com", "GET", "/api/permissions", ""); r.Code != 200 {
		t.Fatalf("read: %d %s", r.Code, r.Body)
	}
}

func TestPolicyRoutesApplyAndRevokeCreationPermissions(t *testing.T) {
	f, chats := permissionRoutes(t)
	member := "member@example.com"
	// Create before the deny, then exercise both new chat and fork after it.
	r := f.do(t, member, "POST", "/api/chats", `{"provider":"codex"}`)
	if r.Code != 201 {
		t.Fatalf("baseline create: %d %s", r.Code, r.Body)
	}
	var original servicechat.Meta
	if err := json.Unmarshal(r.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	for _, key := range []rbac.Key{serviceproject.PermissionCreate, servicechat.PermissionHostCreate} {
		body := fmt.Sprintf(`{"UserEmail":%q,"Permission":%q,"Effect":"deny","Scope":{"kind":"platform"}}`, member, key)
		if r := f.do(t, "admin@example.com", "PUT", "/api/permissions/assignments", body); r.Code != 200 {
			t.Fatalf("deny: %d %s", r.Code, r.Body)
		}
	}
	for _, path := range []string{"/api/chats", "/api/chats/" + string(original.ID) + "/fork", "/api/projects"} {
		if r := f.do(t, member, "POST", path, `{"name":"Denied","provider":"codex"}`); r.Code != 403 {
			t.Fatalf("%s: %d %s", path, r.Code, r.Body)
		}
	}
	all, err := chats.List(context.Background())
	if err != nil || len(all) != 1 {
		t.Fatalf("denied requests changed chats: %v %v", all, err)
	}
	projects, err := f.projects.List(context.Background())
	if err != nil || len(projects) != 1 {
		t.Fatalf("denied create changed projects: %v %v", projects, err)
	}
	r = f.do(t, member, "GET", "/api/permissions/effective", "")
	var effective map[string]bool
	_ = json.Unmarshal(r.Body.Bytes(), &effective)
	if r.Code != 200 || effective[string(servicechat.PermissionHostCreate)] || effective[string(serviceproject.PermissionCreate)] {
		t.Fatalf("effective: %d %s", r.Code, r.Body)
	}
	target := `{"UserEmail":"member@example.com","Permission":"chats.host.create","Scope":{"kind":"platform"}}`
	if r := f.do(t, "admin@example.com", "DELETE", "/api/permissions/assignments", target); r.Code != 200 {
		t.Fatal(r.Body)
	}
	if r := f.do(t, member, "POST", "/api/chats", `{"provider":"codex"}`); r.Code != 201 {
		t.Fatalf("revoked deny: %d %s", r.Code, r.Body)
	}
	// Project checks are independent of the server-scoped host-chat permission.
	r = f.do(t, member, "GET", "/api/permissions/effective?projectId="+f.projectID, "")
	effective = nil
	_ = json.Unmarshal(r.Body.Bytes(), &effective)
	if !effective[string(servicechat.PermissionProjectCreate)] {
		t.Fatalf("project membership: %s", r.Body)
	}
	r = f.do(t, member, "GET", "/api/permissions/effective?projectId=other-project", "")
	effective = nil
	_ = json.Unmarshal(r.Body.Bytes(), &effective)
	if effective[string(servicechat.PermissionProjectCreate)] {
		t.Fatalf("cross-project baseline: %s", r.Body)
	}
}

func TestRoleManagementBindingAndDenyPrecedence(t *testing.T) {
	f, _ := permissionRoutes(t)
	r := f.do(t, "admin@example.com", "POST", "/api/permissions/roles", `{"Name":"Restricted creator","Rules":[{"Permission":"projects.project.create","Effect":"deny"}]}`)
	var role rbac.Role
	_ = json.Unmarshal(r.Body.Bytes(), &role)
	if r.Code != 200 || role.ID == "" {
		t.Fatalf("create role: %d %s", r.Code, r.Body)
	}
	binding := fmt.Sprintf(`{"UserEmail":"member@example.com","RoleID":%q,"Scope":{"kind":"platform"}}`, role.ID)
	if r := f.do(t, "admin@example.com", "PUT", "/api/permissions/bindings", binding); r.Code != 200 {
		t.Fatalf("bind: %d %s", r.Code, r.Body)
	}
	if r := f.do(t, "admin@example.com", "PUT", "/api/permissions/assignments", `{"UserEmail":"member@example.com","Permission":"projects.project.create","Effect":"allow","Scope":{"kind":"platform"}}`); r.Code != 200 {
		t.Fatal(r.Body)
	}
	if r := f.do(t, "member@example.com", "POST", "/api/projects", `{"name":"Denied by role"}`); r.Code != 403 {
		t.Fatalf("deny precedence: %d %s", r.Code, r.Body)
	}
	if r := f.do(t, "admin@example.com", "DELETE", "/api/permissions/roles/"+role.ID, ""); r.Code != 409 {
		t.Fatalf("bound deletion: %d %s", r.Code, r.Body)
	}
	// An administrator cannot be locked out by the same role.
	adminBinding := fmt.Sprintf(`{"UserEmail":"admin@example.com","RoleID":%q,"Scope":{"kind":"platform"}}`, role.ID)
	if r := f.do(t, "admin@example.com", "PUT", "/api/permissions/bindings", adminBinding); r.Code != 200 {
		t.Fatal(r.Body)
	}
	if r := f.do(t, "admin@example.com", "POST", "/api/projects", `{"name":"Admin recovery"}`); r.Code != 201 {
		t.Fatalf("admin recovery: %d %s", r.Code, r.Body)
	}
	for _, body := range []string{binding, adminBinding} {
		if r := f.do(t, "admin@example.com", "DELETE", "/api/permissions/bindings", body); r.Code != 200 {
			t.Fatal(r.Body)
		}
	}
	if r := f.do(t, "admin@example.com", "DELETE", "/api/permissions/roles/"+role.ID, ""); r.Code != 200 {
		t.Fatal(r.Body)
	}
	if r := f.do(t, "member@example.com", "POST", "/api/projects", `{"name":"Allowed again"}`); r.Code != 201 {
		t.Fatalf("unbind: %d %s", r.Code, r.Body)
	}
}

func TestDelegatedManagerCannotEscalateThroughHTTP(t *testing.T) {
	f, _ := permissionRoutes(t)
	// Delegate assignments management only; its own rule is non-delegable.
	if _, err := f.permissions.SetAssignment(adminContext(), rbac.AssignmentInput{UserEmail: "member@example.com", Permission: rbac.PermissionAssignmentsManage, Effect: rbac.Allow, Scope: rbac.PlatformScope()}); err != nil {
		t.Fatal(err)
	}
	if r := f.do(t, "member@example.com", "GET", "/api/permissions", ""); r.Code != 200 {
		t.Fatalf("delegated read: %d %s", r.Code, r.Body)
	}
	if r := f.do(t, "member@example.com", "PUT", "/api/permissions/assignments", `{"UserEmail":"outsider@example.com","Permission":"permissions.assignments.manage","Effect":"allow","Scope":{"kind":"platform"}}`); r.Code != 403 {
		t.Fatalf("escalation: %d %s", r.Code, r.Body)
	}
	if r := f.do(t, "member@example.com", "POST", "/api/permissions/roles", `{"Name":"Escalate","Rules":[{"Permission":"projects.project.create","Effect":"allow"}]}`); r.Code != 403 {
		t.Fatalf("role escalation: %d %s", r.Code, r.Body)
	}
	if _, err := f.permissions.SetAssignment(adminContext(), rbac.AssignmentInput{UserEmail: "member@example.com", Permission: serviceproject.PermissionCreate, Effect: rbac.Deny, Scope: rbac.PlatformScope()}); err != nil {
		t.Fatal(err)
	}
	if r := f.do(t, "member@example.com", "PUT", "/api/permissions/assignments", `{"UserEmail":"outsider@example.com","Permission":"projects.project.create","Effect":"allow","Scope":{"kind":"platform"}}`); r.Code != 403 {
		t.Fatalf("delegate unheld permission: %d %s", r.Code, r.Body)
	}
}
