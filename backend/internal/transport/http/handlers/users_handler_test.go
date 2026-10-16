package httphandlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceemail "github.com/futrx-com/remote.futrx.com/internal/service/email"
	serviceinvitation "github.com/futrx-com/remote.futrx.com/internal/service/invitation"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filesessions"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filetwofactor"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileusers"
)

// usersTestDirectory adapts the real user service to the auth directory port
// so claims and admin checks run against the same persisted users.
type usersTestDirectory struct{ users *serviceuser.Service }

func (d usersTestDirectory) IsAdmin(ctx context.Context, email string) (bool, error) {
	return d.users.IsAdmin(ctx, email)
}
func (d usersTestDirectory) IsRegistered(ctx context.Context, email string) (bool, error) {
	return d.users.IsRegistered(ctx, email)
}
func (d usersTestDirectory) AddBootstrapAdmin(ctx context.Context, email string) error {
	_, err := d.users.Add(ctx, email, serviceuser.RoleAdmin, "")
	return err
}
func (d usersTestDirectory) FirstAdmin(ctx context.Context) (*serviceauth.UserDirectoryEntry, error) {
	list, err := d.users.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, u := range list {
		if u.Role == serviceuser.RoleAdmin {
			return &serviceauth.UserDirectoryEntry{Email: u.Email}, nil
		}
	}
	return nil, nil
}

// usersTestCleanup is what the real removal cleanup does for invited
// credentials; the auth service is bound once it exists.
type usersTestCleanup struct{ auth *serviceauth.Service }

func (c *usersTestCleanup) CleanupRemovedUser(ctx context.Context, email string) error {
	if c.auth == nil {
		return nil
	}
	return c.auth.DeleteAccountCredential(ctx, email)
}

type usersFixture struct {
	t           *testing.T
	auth        *serviceauth.Service
	users       *serviceuser.Service
	invitations *serviceinvitation.Service
	oauth       *usersTestOAuth
	mux         *http.ServeMux
}

// usersTestOAuth answers the Google callback with whichever user the test set.
type usersTestOAuth struct{ user serviceauth.User }

func (*usersTestOAuth) AuthCodeURL(string) string { return "https://accounts.example.com" }
func (o *usersTestOAuth) ExchangeUser(context.Context, string) (serviceauth.User, error) {
	return o.user, nil
}

func newUsersFixture(t *testing.T, google bool) *usersFixture {
	t.Helper()
	root := t.TempDir()
	repo, err := fileusers.New(root)
	if err != nil {
		t.Fatal(err)
	}
	cleanup := &usersTestCleanup{}
	oauth := &usersTestOAuth{}
	users := serviceuser.New(repo, serviceuser.WithRemovalCleanup(cleanup))
	factors, err := filetwofactor.New(root)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := filesessions.New(root)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := serviceauth.New(context.Background(), fileauth.New(root), usersTestDirectory{users},
		func(string, string, string) serviceauth.OAuthProvider { return oauth },
		"https://remote.example.com", []byte("test-session-key"), factors, registry, accountCredentialStoreForTest(t),
		serviceauth.Options{PendingLoginTTL: 5 * time.Minute, EnrollmentTTL: 10 * time.Minute, RecoveryCodeCount: 10, SessionHistoryLimit: 20, SetupTokenTTL: 30 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if google {
		if err := auth.ConfigureGoogleOAuth(context.Background(), serviceauth.OAuthConfig{GoogleClientID: "id", GoogleClientSecret: "secret"}); err != nil {
			t.Fatal(err)
		}
	}
	cleanup.auth = auth
	// A nil mailer reports SMTP as not configured.
	invitations := serviceinvitation.New(serviceinvitation.Dependencies{
		Users: users, Credentials: auth, Policy: auth,
		Mailer: (*serviceemail.Mailer)(nil), BaseURL: "https://remote.example.com",
	})
	mux := http.NewServeMux()
	NewUsersHandler(users, invitations, auth).RegisterRoutes(mux)
	NewAuthHandler(auth, nil).RegisterRoutes(mux)
	NewSecurityHandler(auth).RegisterRoutes(mux)
	return &usersFixture{t: t, auth: auth, users: users, invitations: invitations, oauth: oauth, mux: mux}
}

// session adds email with role and returns a signed session cookie for it.
func (f *usersFixture) session(email string, role serviceuser.Role) *http.Cookie {
	f.t.Helper()
	if _, err := f.users.Add(context.Background(), email, role, ""); err != nil {
		f.t.Fatal(err)
	}
	value, err := f.auth.IssueSession(context.Background(), serviceauth.User{Email: email, Sub: "google-sub"}, serviceauth.SignInMethodGoogle, "", "")
	if err != nil {
		f.t.Fatal(err)
	}
	return &http.Cookie{Name: serviceauth.SessionCookieName, Value: value}
}

func (f *usersFixture) do(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func expectUsersStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d: %s", rec.Code, want, rec.Body.String())
	}
}

func TestUsersHandlerRequiresASession(t *testing.T) {
	f := newUsersFixture(t, true)
	expectUsersStatus(t, f.do(http.MethodGet, "/api/admin/users", "", nil), http.StatusUnauthorized)
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users", `{"email":"a@example.com","role":"member"}`, nil), http.StatusUnauthorized)
	expectUsersStatus(t, f.do(http.MethodDelete, "/api/admin/users/a@example.com", "", nil), http.StatusUnauthorized)
}

func TestUsersHandlerRejectsNonAdmins(t *testing.T) {
	f := newUsersFixture(t, true)
	member := f.session("member@example.com", serviceuser.RoleMember)
	expectUsersStatus(t, f.do(http.MethodGet, "/api/admin/users", "", member), http.StatusForbidden)
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users", `{"email":"a@example.com","role":"member"}`, member), http.StatusForbidden)
	expectUsersStatus(t, f.do(http.MethodDelete, "/api/admin/users/member@example.com", "", member), http.StatusForbidden)
}

func TestUsersHandlerRefusesToAddWithoutGoogle(t *testing.T) {
	f := newUsersFixture(t, false)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	rec := f.do(http.MethodPost, "/api/admin/users", `{"email":"new@example.com","role":"member"}`, admin)
	expectUsersStatus(t, rec, http.StatusConflict)
	if !strings.Contains(rec.Body.String(), "configure Google sign-in before adding users") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if got, _ := f.users.Get(context.Background(), "new@example.com"); got != nil {
		t.Fatalf("user was created despite refusal: %+v", got)
	}
}

func TestUsersHandlerAddsAndRejectsDuplicates(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)

	rec := f.do(http.MethodPost, "/api/admin/users", `{"email":" New@Example.com ","role":"member"}`, admin)
	expectUsersStatus(t, rec, http.StatusCreated)
	got := decodeJSON[serviceuser.User](t, rec)
	if got.Email != "new@example.com" || got.Role != serviceuser.RoleMember || got.AddedBy != "admin@example.com" {
		t.Fatalf("created = %+v", got)
	}

	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users", `{"email":"new@example.com","role":"admin"}`, admin), http.StatusConflict)
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users", `{"email":"x@example.com","role":"owner"}`, admin), http.StatusBadRequest)
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users", `{`, admin), http.StatusBadRequest)
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users", `{"email":"nope","role":"member"}`, admin), http.StatusBadRequest)
}

func TestUsersHandlerChangesRoles(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users", `{"email":"new@example.com","role":"member"}`, admin), http.StatusCreated)

	rec := f.do(http.MethodPut, "/api/admin/users/new@example.com/role", `{"role":"admin"}`, admin)
	expectUsersStatus(t, rec, http.StatusOK)
	if got := decodeJSON[serviceuser.User](t, rec); got.Role != serviceuser.RoleAdmin {
		t.Fatalf("role = %q", got.Role)
	}
	expectUsersStatus(t, f.do(http.MethodPut, "/api/admin/users/missing@example.com/role", `{"role":"admin"}`, admin), http.StatusNotFound)
	expectUsersStatus(t, f.do(http.MethodPut, "/api/admin/users/new@example.com/role", `{"role":"owner"}`, admin), http.StatusBadRequest)
	expectUsersStatus(t, f.do(http.MethodGet, "/api/admin/users/new@example.com/role", "", admin), http.StatusMethodNotAllowed)
}

func TestUsersHandlerListsAndRemoves(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users", `{"email":"new@example.com","role":"member"}`, admin), http.StatusCreated)

	rec := f.do(http.MethodGet, "/api/admin/users", "", admin)
	expectUsersStatus(t, rec, http.StatusOK)
	if got := decodeJSON[[]serviceuser.User](t, rec); len(got) != 2 {
		t.Fatalf("list = %+v", got)
	}
	expectUsersStatus(t, f.do(http.MethodDelete, "/api/admin/users/new@example.com", "", admin), http.StatusOK)
	expectUsersStatus(t, f.do(http.MethodDelete, "/api/admin/users/new@example.com", "", admin), http.StatusNotFound)
}

func TestUsersHandlerProtectsTheLastAdmin(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	expectUsersStatus(t, f.do(http.MethodPut, "/api/admin/users/admin@example.com/role", `{"role":"member"}`, admin), http.StatusConflict)
	expectUsersStatus(t, f.do(http.MethodDelete, "/api/admin/users/admin@example.com", "", admin), http.StatusConflict)
}

func TestUsersHandlerProtectsTheLocalAdministrator(t *testing.T) {
	f := newUsersFixture(t, true)
	const localEmail = "owner@example.com"
	const localPassword = "local admin password"
	if _, err := f.auth.ClaimLocalAdmin(context.Background(), serviceauth.ClaimRequest{Email: localEmail, Password: localPassword, SetupToken: setupTokenFor(t, f.auth)}); err != nil {
		t.Fatal(err)
	}
	// A second administrator makes the last-admin rule irrelevant, so the
	// refusals below can only come from the local-admin guard.
	other := f.session("other@example.com", serviceuser.RoleAdmin)

	rec := f.do(http.MethodPut, "/api/admin/users/owner@example.com/role", `{"role":"member"}`, other)
	expectUsersStatus(t, rec, http.StatusConflict)
	if !strings.Contains(rec.Body.String(), "the local administrator cannot be demoted") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	rec = f.do(http.MethodDelete, "/api/admin/users/owner@example.com", "", other)
	expectUsersStatus(t, rec, http.StatusConflict)
	if !strings.Contains(rec.Body.String(), "the local administrator cannot be removed") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	// Keeping the local administrator an admin is allowed.
	expectUsersStatus(t, f.do(http.MethodPut, "/api/admin/users/owner@example.com/role", `{"role":"admin"}`, other), http.StatusOK)
}
