package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	servicepermission "github.com/futrx-com/remote.futrx.com/internal/rbac"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
)

type permissionsRoutesFixture struct {
	handler     http.Handler
	auth        *serviceauth.Service
	permissions *servicepermission.Service
	roleID      string
}

// newPermissionsRoutesFixture serves the real permission service behind the
// real auth middleware, with one role already defined.
func newPermissionsRoutesFixture(t *testing.T) permissionsRoutesFixture {
	t.Helper()
	base := newMembershipRoutesFixture(t)
	mux := http.NewServeMux()
	NewPermissionsHandler(base.permissions).RegisterRoutes(mux)
	role, err := base.permissions.CreateRole(adminContext(), servicepermission.RoleInput{
		Name: "Access managers",
		Rules: []servicepermission.RoleRule{
			{Permission: serviceproject.PermissionAccessManage, Effect: servicepermission.Allow},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return permissionsRoutesFixture{
		handler:     httpmiddleware.NewAuth(base.auth).Wrap(mux),
		auth:        base.auth,
		permissions: base.permissions,
		roleID:      role.ID,
	}
}

func (f permissionsRoutesFixture) do(t *testing.T, email, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if email != "" {
		session, err := f.auth.IssueSession(
			context.Background(), serviceauth.User{Email: email}, serviceauth.SignInMethodPassword, "", "",
		)
		if err != nil {
			t.Fatalf("issue session: %v", err)
		}
		request.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: session})
	}
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

const (
	permissionsBase      = "/api/admin/permissions"
	assignmentJSON       = `{"userEmail":"member@example.com","permission":"projects.access.manage","effect":"allow","scope":{"kind":"project","id":"proj1"}}`
	assignmentDeletePath = permissionsBase + "/assignments?userEmail=member@example.com&permission=projects.access.manage&scopeKind=project&scopeId=proj1"
	roleJSON             = `{"name":"Lifecycle","description":"d","rules":[{"permission":"projects.lifecycle.manage","effect":"allow"}]}`
)

func (f permissionsRoutesFixture) bindingJSON() string {
	return `{"userEmail":"member@example.com","roleId":"` + f.roleID + `","scope":{"kind":"project","id":"proj1"}}`
}

func (f permissionsRoutesFixture) bindingDeletePath() string {
	return permissionsBase + "/bindings?userEmail=member@example.com&roleId=" + f.roleID + "&scopeKind=project&scopeId=proj1"
}

type permissionsRoute struct {
	name   string
	method string
	path   string
	body   string
	want   int
}

func (f permissionsRoutesFixture) routes() []permissionsRoute {
	return []permissionsRoute{
		{"list definitions", http.MethodGet, permissionsBase + "/definitions", "", http.StatusOK},
		{"list roles", http.MethodGet, permissionsBase + "/roles", "", http.StatusOK},
		{"list assignments", http.MethodGet, permissionsBase + "/assignments", "", http.StatusOK},
		{"list bindings", http.MethodGet, permissionsBase + "/bindings", "", http.StatusOK},
		{"set assignment", http.MethodPost, permissionsBase + "/assignments", assignmentJSON, http.StatusOK},
		{"remove assignment", http.MethodDelete, assignmentDeletePath, "", http.StatusNoContent},
		{"bind role", http.MethodPost, permissionsBase + "/bindings", f.bindingJSON(), http.StatusOK},
		{"unbind role", http.MethodDelete, f.bindingDeletePath(), "", http.StatusNoContent},
		{"create role", http.MethodPost, permissionsBase + "/roles", roleJSON, http.StatusOK},
		{"update role", http.MethodPut, permissionsBase + "/roles/" + f.roleID, strings.Replace(roleJSON, "Lifecycle", "Renamed", 1), http.StatusOK},
		{"delete role", http.MethodDelete, permissionsBase + "/roles/" + f.roleID, "", http.StatusNoContent},
	}
}

func TestPermissionsRoutesAllowAdministrator(t *testing.T) {
	fixture := newPermissionsRoutesFixture(t)
	for _, route := range fixture.routes() {
		response := fixture.do(t, "admin@example.com", route.method, route.path, route.body)
		if response.Code != route.want {
			t.Fatalf("%s: status = %d, want %d: %s", route.name, response.Code, route.want, response.Body)
		}
	}
}

func TestPermissionsRoutesDenyOrdinaryMembersWith403(t *testing.T) {
	fixture := newPermissionsRoutesFixture(t)
	for _, route := range fixture.routes() {
		response := fixture.do(t, "member@example.com", route.method, route.path, route.body)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403: %s", route.name, response.Code, response.Body)
		}
	}
}

func TestPermissionsRoutesRejectAnonymousCallers(t *testing.T) {
	fixture := newPermissionsRoutesFixture(t)
	response := fixture.do(t, "", http.MethodGet, permissionsBase+"/roles", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestPermissionsRoutesRejectMalformedInputWith400(t *testing.T) {
	fixture := newPermissionsRoutesFixture(t)
	for _, route := range []permissionsRoute{
		{"assignment json", http.MethodPost, permissionsBase + "/assignments", "{", 0},
		{"assignment unknown permission", http.MethodPost, permissionsBase + "/assignments",
			`{"userEmail":"member@example.com","permission":"nope.nope.nope","effect":"allow","scope":{"kind":"platform"}}`, 0},
		{"assignment bad effect", http.MethodPost, permissionsBase + "/assignments",
			strings.Replace(assignmentJSON, `"allow"`, `"maybe"`, 1), 0},
		{"assignment bad scope", http.MethodPost, permissionsBase + "/assignments",
			strings.Replace(assignmentJSON, `"project"`, `"galaxy"`, 1), 0},
		{"assignment unregistered user", http.MethodPost, permissionsBase + "/assignments",
			strings.Replace(assignmentJSON, "member@example.com", "ghost@example.com", 1), 0},
		{"assignment delete without keys", http.MethodDelete, permissionsBase + "/assignments", "", 0},
		{"binding json", http.MethodPost, permissionsBase + "/bindings", "{", 0},
		{"binding scope unsupported by role", http.MethodPost, permissionsBase + "/bindings",
			`{"userEmail":"member@example.com","roleId":"` + fixture.roleID + `","scope":{"kind":"platform"}}`, 0},
		{"binding delete without keys", http.MethodDelete, permissionsBase + "/bindings", "", 0},
		{"role json", http.MethodPost, permissionsBase + "/roles", "{", 0},
		{"role without rules", http.MethodPost, permissionsBase + "/roles", `{"name":"Empty","rules":[]}`, 0},
		{"role update json", http.MethodPut, permissionsBase + "/roles/" + fixture.roleID, "{", 0},
	} {
		response := fixture.do(t, "admin@example.com", route.method, route.path, route.body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400: %s", route.name, response.Code, response.Body)
		}
	}
}

func TestPermissionsRepeatedWritesReturn200(t *testing.T) {
	fixture := newPermissionsRoutesFixture(t)
	for _, route := range []permissionsRoute{
		{"assignment", http.MethodPost, permissionsBase + "/assignments", assignmentJSON, 0},
		{"binding", http.MethodPost, permissionsBase + "/bindings", fixture.bindingJSON(), 0},
	} {
		var ids []string
		for attempt := 0; attempt < 2; attempt++ {
			response := fixture.do(t, "admin@example.com", route.method, route.path, route.body)
			if response.Code != http.StatusOK {
				t.Fatalf("%s attempt %d: status = %d, want 200: %s", route.name, attempt, response.Code, response.Body)
			}
			var record struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, record.ID)
		}
		if ids[0] == "" || ids[0] != ids[1] {
			t.Fatalf("%s: repeated write ids = %v, want the same record", route.name, ids)
		}
	}
}

func TestPermissionsDifferentEffectUpdatesTheAssignment(t *testing.T) {
	fixture := newPermissionsRoutesFixture(t)
	fixture.do(t, "admin@example.com", http.MethodPost, permissionsBase+"/assignments", assignmentJSON)
	denied := strings.Replace(assignmentJSON, `"allow"`, `"deny"`, 1)
	response := fixture.do(t, "admin@example.com", http.MethodPost, permissionsBase+"/assignments", denied)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"effect":"deny"`) {
		t.Fatalf("status = %d body = %s, want 200 with the deny effect", response.Code, response.Body)
	}
	list := fixture.do(t, "admin@example.com", http.MethodGet, permissionsBase+"/assignments", "")
	if strings.Count(list.Body.String(), `"userEmail"`) != 1 {
		t.Fatalf("assignments = %s, want one record", list.Body)
	}
}

func TestPermissionsDeleteBoundRoleNeedsUnbind(t *testing.T) {
	fixture := newPermissionsRoutesFixture(t)
	fixture.do(t, "admin@example.com", http.MethodPost, permissionsBase+"/bindings", fixture.bindingJSON())
	path := permissionsBase + "/roles/" + fixture.roleID
	if got := fixture.do(t, "admin@example.com", http.MethodDelete, path, "").Code; got != http.StatusConflict {
		t.Fatalf("delete bound role status = %d, want 409", got)
	}
	if got := fixture.do(t, "admin@example.com", http.MethodDelete, path+"?unbind=true", "").Code; got != http.StatusNoContent {
		t.Fatalf("delete with unbind status = %d, want 204", got)
	}
}

func TestPermissionsListsUseEmptyArraysAndTaggedFields(t *testing.T) {
	fixture := newPermissionsRoutesFixture(t)
	assignments := fixture.do(t, "admin@example.com", http.MethodGet, permissionsBase+"/assignments", "")
	if strings.TrimSpace(assignments.Body.String()) != `{"assignments":[]}` {
		t.Fatalf("assignments = %s, want an empty array", assignments.Body)
	}
	definitions := fixture.do(t, "admin@example.com", http.MethodGet, permissionsBase+"/definitions", "")
	for _, field := range []string{`"key"`, `"description"`, `"scopes"`, `"baseline"`, `"delegable"`} {
		if !strings.Contains(definitions.Body.String(), field) {
			t.Fatalf("definitions = %s, missing %s", definitions.Body, field)
		}
	}
	roles := fixture.do(t, "admin@example.com", http.MethodGet, permissionsBase+"/roles", "")
	for _, field := range []string{`"id"`, `"name"`, `"rules"`, `"createdBy"`, `"createdAt"`, `"updatedAt"`} {
		if !strings.Contains(roles.Body.String(), field) {
			t.Fatalf("roles = %s, missing %s", roles.Body, field)
		}
	}
}

func TestPermissionsDefinitionsAreReadOnly(t *testing.T) {
	fixture := newPermissionsRoutesFixture(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		response := fixture.do(t, "admin@example.com", method, permissionsBase+"/definitions", "{}")
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s definitions status = %d, want 405", method, response.Code)
		}
	}
}
