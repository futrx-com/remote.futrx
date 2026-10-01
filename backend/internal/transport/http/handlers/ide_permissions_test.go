package httphandlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspaceide"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
)

func TestIDEForwardAuthEnforcesCurrentPolicyForEveryRoutedURL(t *testing.T) {
	f := newMembershipRoutesFixture(t)
	project, err := f.projects.Get(context.Background(), serviceproject.ID(f.projectID))
	if err != nil {
		t.Fatal(err)
	}
	h := &authVerifyHandler{auth: f.auth, access: serviceauth.NewAccessVerifier(f.auth, f.projects).WithIDEAuthorizer(f.permissions), shares: &shareAuthorizerStub{validToken: "shared", allows: true}}
	cookie := func(email string) *http.Cookie {
		value, err := f.auth.IssueSession(context.Background(), serviceauth.User{Email: email}, serviceauth.SignInMethodPassword, "", "")
		if err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: serviceauth.SessionCookieName, Value: value}
	}
	member, admin, outsider := cookie("member@example.com"), cookie("admin@example.com"), cookie("outsider@example.com")
	routes := []struct{ host, uri string }{
		{"code." + verifyBaseHost, "/" + project.Slug + "/?folder=/workspace"},
		{"code." + verifyBaseHost, "/" + project.Slug},
		{project.Slug + ".code." + verifyBaseHost, "/"},
		{project.Slug + "--8842.dev." + verifyBaseHost, "/?share=shared"},
		{project.Slug + "--8081.dev." + verifyBaseHost, "/"},
	}
	for _, route := range routes {
		for _, test := range []struct {
			cookie *http.Cookie
			status int
		}{{nil, 302}, {member, 200}, {admin, 200}, {outsider, 403}} {
			r := verifyRequest(t, h, route.host, route.uri, test.cookie)
			if r.Code != test.status {
				t.Fatalf("%s%s: %d want %d: %s", route.host, route.uri, r.Code, test.status, r.Body)
			}
		}
	}
	if _, err := f.permissions.SetAssignment(adminContext(), rbac.AssignmentInput{UserEmail: "member@example.com", Permission: workspaceide.PermissionProjectOpen, Effect: rbac.Deny, Scope: rbac.ProjectScope(f.projectID)}); err != nil {
		t.Fatal(err)
	}
	// Reuse the same session; revocation does not depend on signing in again.
	for _, route := range routes {
		if r := verifyRequest(t, h, route.host, route.uri, member); r.Code != 403 {
			t.Fatalf("revoked %s%s: %d %s", route.host, route.uri, r.Code, r.Body)
		}
		if r := verifyRequest(t, h, route.host, route.uri, admin); r.Code != 200 {
			t.Fatalf("admin %s: %d", route.host, r.Code)
		}
	}
	if r := verifyRequest(t, h, "missing-project.code."+verifyBaseHost, "/", member); r.Code != 404 {
		t.Fatalf("missing: %d", r.Code)
	}
	h.access = serviceauth.NewAccessVerifier(f.auth, f.projects)
	if r := verifyRequest(t, h, routes[0].host, routes[0].uri, member); r.Code != 403 {
		t.Fatalf("missing authorizer: %d", r.Code)
	}
}

func TestIDETargetParsing(t *testing.T) {
	h, _ := newVerifyHandler(t)
	for _, test := range []struct {
		host, uri, slug string
		ide, invalid    bool
	}{
		{"code." + verifyBaseHost, "/", "", false, false},
		{"code." + verifyBaseHost, "/api/projects", "", false, false},
		{"code." + verifyBaseHost, "/sw.js", "", false, false},
		{"code." + verifyBaseHost, "/alpha/", "alpha", true, false},
		{"code." + verifyBaseHost, "/alpha%2fworkbench", "alpha", true, false},
		{"alpha.code." + verifyBaseHost, "/anything", "alpha", true, false},
		{"alpha.code.attacker.test", "/", "", false, false},
		{"code." + verifyBaseHost, "/alpha/../beta/", "", true, true},
		{"code." + verifyBaseHost, "/alpha/%2e%2e/beta/", "", true, true},
		{"code." + verifyBaseHost, "//alpha/", "", true, true},
		{"nested.alpha.code." + verifyBaseHost, "/", "", true, true},
	} {
		// verifyRequest constructs exactly these forwarded headers; use the parser
		// separately to assert launcher assets remain under ordinary session auth.
		req := httptest.NewRequest("GET", "/auth/verify", nil)
		req.Header.Set("X-Forwarded-Host", test.host)
		req.Header.Set("X-Forwarded-Uri", test.uri)
		slug, ide, err := h.ideTarget(req, test.host)
		if slug != test.slug || ide != test.ide || (err != nil) != test.invalid {
			t.Fatalf("%s%s: %q %v %v", test.host, test.uri, slug, ide, err)
		}
	}
}

func TestIDEOpenRouteUsesSamePermissionAsDirectNavigation(t *testing.T) {
	f, chats := permissionRoutes(t)
	project, err := f.projects.Get(context.Background(), serviceproject.ID(f.projectID))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := chats.Create(adminContext(), servicechat.CreateInput{Provider: servicechat.ProviderCodex, ProjectID: servicechat.ProjectID(f.projectID), Cwd: project.Cwd})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	ide := workspaceide.New("https://code."+verifyBaseHost+"/", "/unused", workspaceide.WithAuthorizer(f.permissions))
	NewChatHandler(chats, servicechat.NewAccessService(chats, f.projects), f.auth, nil, nil, ide).RegisterRoutes(mux)
	f.handler = httpmiddleware.NewAuth(f.auth).Wrap(mux)
	url := "/api/chats/" + string(meta.ID) + "/ide-open?path=/workspace"
	if r := f.do(t, "member@example.com", "GET", url, ""); r.Code != 302 {
		t.Fatalf("baseline: %d %s", r.Code, r.Body)
	}
	role, err := f.permissions.CreateRole(adminContext(), rbac.RoleInput{Name: "No IDE", Rules: []rbac.RoleRule{{Permission: workspaceide.PermissionProjectOpen, Effect: rbac.Deny}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.permissions.BindRole(adminContext(), rbac.BindingInput{RoleID: role.ID, UserEmail: "member@example.com", Scope: rbac.ProjectScope(f.projectID)}); err != nil {
		t.Fatal(err)
	}
	if r := f.do(t, "member@example.com", "GET", url, ""); r.Code != 403 || r.Header().Get("Location") != "" {
		t.Fatalf("role deny: %d %s", r.Code, r.Body)
	}
	if r := f.do(t, "admin@example.com", "GET", url, ""); r.Code != 302 {
		t.Fatalf("admin: %d %s", r.Code, r.Body)
	}
}

func TestBrowserForwardAuthUsesCurrentPolicyAndRejectsSharedNoVNC(t *testing.T) {
	f := newMembershipRoutesFixture(t)
	project, err := f.projects.Get(context.Background(), serviceproject.ID(f.projectID))
	if err != nil {
		t.Fatal(err)
	}
	h := &authVerifyHandler{auth: f.auth, access: serviceauth.NewAccessVerifier(f.auth, f.projects).WithIDEAuthorizer(f.permissions), shares: &shareAuthorizerStub{validToken: "shared", allows: true}}
	value, err := f.auth.IssueSession(context.Background(), serviceauth.User{Email: "member@example.com"}, serviceauth.SignInMethodPassword, "", "")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: serviceauth.SessionCookieName, Value: value}
	host := project.Slug + "--6080.dev." + verifyBaseHost
	if r := verifyRequest(t, h, host, "/?share=shared", nil); r.Code != 302 {
		t.Fatalf("unauthenticated noVNC %d", r.Code)
	}
	if r := verifyRequest(t, h, host, "/", cookie); r.Code != 200 {
		t.Fatalf("member %d %s", r.Code, r.Body)
	}
	_, err = f.permissions.SetAssignment(adminContext(), rbac.AssignmentInput{UserEmail: "member@example.com", Permission: "workspace.browser.use", Effect: rbac.Deny, Scope: rbac.ProjectScope(f.projectID)})
	if err != nil {
		t.Fatal(err)
	}
	if r := verifyRequest(t, h, host, "/?share=shared", cookie); r.Code != 403 {
		t.Fatalf("revoked noVNC %d", r.Code)
	}
	if r := verifyRequest(t, h, project.Slug+"--3000.dev."+verifyBaseHost, "/", cookie); r.Code != 403 {
		t.Fatalf("revoked preview %d", r.Code)
	}
}
