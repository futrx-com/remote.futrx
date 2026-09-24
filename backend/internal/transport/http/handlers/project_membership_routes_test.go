package httphandlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicepermission "github.com/futrx-com/remote.futrx.com/internal/service/permission"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filepermissions"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileprojectaccess"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
)

// membershipUserDirectory is an in-memory user directory: the map value is
// whether the account is an administrator.
type membershipUserDirectory map[string]bool

func (d membershipUserDirectory) IsAdmin(_ context.Context, email string) (bool, error) {
	return d[email], nil
}

func (d membershipUserDirectory) IsRegistered(_ context.Context, email string) (bool, error) {
	_, ok := d[email]
	return ok, nil
}

func (membershipUserDirectory) AddBootstrapAdmin(context.Context, string) error { return nil }

func (membershipUserDirectory) FirstAdmin(context.Context) (*serviceauth.UserDirectoryEntry, error) {
	return nil, nil
}

type membershipRoutesFixture struct {
	handler     http.Handler
	auth        *serviceauth.Service
	projects    *serviceproject.Service
	permissions *servicepermission.Service
	dataDir     string
	projectID   string
}

// membershipAccess adapts the project access repository to the permission
// layer's membership port, as the composition root does.
type membershipAccess struct {
	access serviceproject.AccessRepository
}

func (m membershipAccess) HasAccess(ctx context.Context, projectID, email string) (bool, error) {
	return m.access.Has(ctx, serviceproject.ID(projectID), servicepermission.NormalizeEmail(email))
}

// newMembershipRoutesFixture wires the project routes the way production
// does: the auth middleware attaches the actor, and the project service
// authorizes through the real permission service over a file store.
func newMembershipRoutesFixture(t *testing.T) membershipRoutesFixture {
	t.Helper()
	repo, err := fileproject.NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	access, err := fileprojectaccess.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	auth, err := serviceauth.New(
		context.Background(),
		fileauth.New(t.TempDir()),
		membershipUserDirectory{
			"admin@example.com":    true,
			"member@example.com":   false,
			"outsider@example.com": false,
		},
		func(string, string, string) serviceauth.OAuthProvider { return verifyOAuthProvider{} },
		"https://"+verifyBaseHost,
		[]byte("membership-routes-test-key"),
		twoFactorStoreForTest(t),
		sessionRegistryStoreForTest(t),
		testAuthOptions(),
	)
	if err != nil {
		t.Fatalf("New auth service: %v", err)
	}
	dataDir := t.TempDir()
	permissions := newMembershipPermissions(t, dataDir, auth, access)
	projects := serviceproject.New(
		repo, serviceproject.ContainerDependencies{}, nil, access,
		serviceproject.WithAuthorizer(permissions),
	)
	project, err := projects.Create(
		context.Background(), serviceproject.CreateInput{Name: "Members Only"}, "member@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewProjectHandler(projects, nil, auth, sharesPublicHostname, nil).RegisterRoutes(mux)
	return membershipRoutesFixture{
		handler:     httpmiddleware.NewAuth(auth).Wrap(mux),
		auth:        auth,
		projects:    projects,
		permissions: permissions,
		dataDir:     dataDir,
		projectID:   string(project.ID),
	}
}

func newMembershipPermissions(
	t *testing.T,
	dataDir string,
	auth *serviceauth.Service,
	access serviceproject.AccessRepository,
) *servicepermission.Service {
	t.Helper()
	store, err := filepermissions.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := servicepermission.NewRegistry(
		servicepermission.ManagementDefinitions(), serviceproject.PermissionDefinitions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	service, err := servicepermission.NewService(
		context.Background(), registry, store, auth, membershipAccess{access: access},
	)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func (f membershipRoutesFixture) do(t *testing.T, email, method, path, body string) *httptest.ResponseRecorder {
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

// These routes were protected only by the handler's admin-or-member check
// before authorization moved into the project service. With an empty
// permission store the visible behavior must be exactly that.
func TestProjectLifecycleAndAccessRoutesRequireMembershipOrAdministrator(t *testing.T) {
	fixture := newMembershipRoutesFixture(t)
	base := "/api/projects/" + fixture.projectID

	routes := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"start", http.MethodPost, base + "/start", ""},
		{"stop", http.MethodPost, base + "/stop", ""},
		{"restart", http.MethodPost, base + "/restart", ""},
		{"repair network", http.MethodPost, base + "/repair-network", ""},
		{"list access", http.MethodGet, base + "/access", ""},
	}
	callers := []struct {
		name       string
		email      string
		wantStatus int
	}{
		{"member", "member@example.com", http.StatusOK},
		{"administrator", "admin@example.com", http.StatusOK},
		{"registered non-member", "outsider@example.com", http.StatusForbidden},
	}
	for _, route := range routes {
		for _, caller := range callers {
			t.Run(route.name+"/"+caller.name, func(t *testing.T) {
				response := fixture.do(t, caller.email, route.method, route.path, route.body)
				if response.Code != caller.wantStatus {
					t.Fatalf("status = %d, want %d, body = %s", response.Code, caller.wantStatus, response.Body.String())
				}
			})
		}
	}
}

func TestProjectAccessMutationRoutesRequireMembershipOrAdministrator(t *testing.T) {
	fixture := newMembershipRoutesFixture(t)
	base := "/api/projects/" + fixture.projectID + "/access"

	forbidden := fixture.do(t, "outsider@example.com", http.MethodPost, base, `{"email":"outsider@example.com"}`)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("non-member add status = %d, want %d", forbidden.Code, http.StatusForbidden)
	}

	added := fixture.do(t, "member@example.com", http.MethodPost, base, `{"email":"outsider@example.com"}`)
	if added.Code != http.StatusOK {
		t.Fatalf("member add status = %d, body = %s", added.Code, added.Body.String())
	}
	removed := fixture.do(t, "admin@example.com", http.MethodDelete, base+"/outsider@example.com", "")
	if removed.Code != http.StatusOK {
		t.Fatalf("administrator remove status = %d, body = %s", removed.Code, removed.Body.String())
	}
}

func TestProjectRoutesRejectAnonymousCallers(t *testing.T) {
	fixture := newMembershipRoutesFixture(t)

	response := fixture.do(t, "", http.MethodPost, "/api/projects/"+fixture.projectID+"/start", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func adminContext() context.Context {
	return servicepermission.ContextWithActor(
		context.Background(), servicepermission.UserActor("admin@example.com"),
	)
}

// A project-scoped deny stops the operation even though the handler's
// membership check passes; the denial comes from the service.
func TestProjectScopedDenyBlocksRoutesThatPassTheMembershipCheck(t *testing.T) {
	fixture := newMembershipRoutesFixture(t)
	base := "/api/projects/" + fixture.projectID
	for _, key := range []servicepermission.Key{
		serviceproject.PermissionLifecycleManage, serviceproject.PermissionAccessManage,
	} {
		if _, err := fixture.permissions.SetAssignment(adminContext(), servicepermission.AssignmentInput{
			UserEmail: "member@example.com", Permission: key, Effect: servicepermission.Deny,
			Scope: servicepermission.ProjectScope(fixture.projectID),
		}); err != nil {
			t.Fatal(err)
		}
	}

	for _, route := range []struct{ method, path string }{
		{http.MethodPost, base + "/start"},
		{http.MethodPost, base + "/stop"},
		{http.MethodPost, base + "/restart"},
		{http.MethodPost, base + "/repair-network"},
		{http.MethodGet, base + "/access"},
	} {
		response := fixture.do(t, "member@example.com", route.method, route.path, "")
		if response.Code == http.StatusOK {
			t.Fatalf("%s %s status = 200, want the service to deny the member", route.method, route.path)
		}
		if !strings.Contains(response.Body.String(), servicepermission.ErrDenied.Error()) {
			t.Fatalf("%s %s body = %s, want a permission denial", route.method, route.path, response.Body.String())
		}
	}
	added := fixture.do(t, "member@example.com", http.MethodPost, base+"/access", `{"email":"outsider@example.com"}`)
	if added.Code == http.StatusOK {
		t.Fatal("a denied member added access")
	}
	members, err := fixture.projects.ListAccess(adminContext(), serviceproject.ID(fixture.projectID))
	if err != nil || len(members) != 1 {
		t.Fatalf("members = %v, %v; want the list untouched", members, err)
	}

	// Other members and administrators are unaffected, and so is the same
	// member on other routes.
	if got := fixture.do(t, "admin@example.com", http.MethodPost, base+"/start", "").Code; got != http.StatusOK {
		t.Fatalf("administrator start status = %d, want %d", got, http.StatusOK)
	}
}

// Skipping HTTP does not skip authorization: a direct service call receives
// the same decision.
func TestDirectServiceCallsReceiveTheSameDecision(t *testing.T) {
	fixture := newMembershipRoutesFixture(t)
	id := serviceproject.ID(fixture.projectID)
	memberContext := servicepermission.ContextWithActor(
		context.Background(), servicepermission.UserActor("member@example.com"),
	)
	outsiderContext := servicepermission.ContextWithActor(
		context.Background(), servicepermission.UserActor("outsider@example.com"),
	)

	if _, err := fixture.projects.Start(memberContext, id); err != nil {
		t.Fatalf("member Start() error = %v, want the empty-policy baseline to allow it", err)
	}
	if _, err := fixture.projects.Start(outsiderContext, id); !errors.Is(err, servicepermission.ErrDenied) {
		t.Fatalf("non-member Start() error = %v, want ErrDenied", err)
	}
	if _, err := fixture.projects.Start(context.Background(), id); !errors.Is(err, servicepermission.ErrActorRequired) {
		t.Fatalf("Start() without an actor error = %v, want ErrActorRequired", err)
	}
	if _, err := fixture.projects.ListAccess(context.Background(), id); !errors.Is(err, servicepermission.ErrActorRequired) {
		t.Fatalf("ListAccess() without an actor error = %v, want ErrActorRequired", err)
	}

	if _, err := fixture.permissions.SetAssignment(adminContext(), servicepermission.AssignmentInput{
		UserEmail: "member@example.com", Permission: serviceproject.PermissionLifecycleManage,
		Effect: servicepermission.Deny, Scope: servicepermission.ProjectScope(fixture.projectID),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.projects.Stop(memberContext, id); !errors.Is(err, servicepermission.ErrDenied) {
		t.Fatalf("denied member Stop() error = %v, want ErrDenied", err)
	}
	// Access management is a separate permission and stays allowed.
	if _, err := fixture.projects.ListAccess(memberContext, id); err != nil {
		t.Fatalf("ListAccess() error = %v, want the other permission unaffected", err)
	}

	// A restarted process reloads the same policy from disk.
	access, err := fileprojectaccess.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	restarted := newMembershipPermissions(t, fixture.dataDir, fixture.auth, access)
	if err := restarted.Require(memberContext, servicepermission.Check{
		Permission: serviceproject.PermissionLifecycleManage,
		Scope:      servicepermission.ProjectScope(fixture.projectID),
	}); !errors.Is(err, servicepermission.ErrDenied) {
		t.Fatalf("after restart error = %v, want ErrDenied", err)
	}
}

// Explicit allow adds a capability without touching membership visibility:
// the handler still refuses a non-member before the service is reached.
func TestExplicitAllowDoesNotBypassTheHandlersMembershipCheck(t *testing.T) {
	fixture := newMembershipRoutesFixture(t)
	if _, err := fixture.permissions.SetAssignment(adminContext(), servicepermission.AssignmentInput{
		UserEmail: "outsider@example.com", Permission: serviceproject.PermissionLifecycleManage,
		Effect: servicepermission.Allow, Scope: servicepermission.ProjectScope(fixture.projectID),
	}); err != nil {
		t.Fatal(err)
	}

	response := fixture.do(t, "outsider@example.com", http.MethodPost, "/api/projects/"+fixture.projectID+"/start", "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}
