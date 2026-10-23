package httphandlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
)

// logCapture records everything logged through slog or the log package.
type logCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *logCapture) Enabled(context.Context, slog.Level) bool { return true }
func (c *logCapture) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&c.buf, " %s=%v", a.Key, a.Value)
		return true
	})
	c.buf.WriteByte('\n')
	return nil
}
func (c *logCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *logCapture) WithGroup(string) slog.Handler      { return c }
func (c *logCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func captureLogs(t *testing.T) *logCapture {
	t.Helper()
	capture := &logCapture{}
	old := slog.Default()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() { slog.SetDefault(old) })
	return capture
}

type inviteResultBody struct {
	User              serviceuser.User `json:"user"`
	Method            string           `json:"method"`
	Delivery          string           `json:"delivery"`
	DeliveryError     string           `json:"deliveryError"`
	TemporaryPassword string           `json:"temporaryPassword"`
}

func (f *usersFixture) invite(admin *http.Cookie, body string) inviteResultBody {
	f.t.Helper()
	rec := f.do(http.MethodPost, "/api/admin/users", body, admin)
	expectUsersStatus(f.t, rec, http.StatusCreated)
	return decodeJSON[inviteResultBody](f.t, rec)
}

func TestPasswordInviteReturnsCredentialsWithoutCaching(t *testing.T) {
	// Google is not configured: a password invite must not need it.
	f := newUsersFixture(t, false)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)

	rec := f.do(http.MethodPost, "/api/admin/users", `{"email":" New@Example.com ","role":"member","method":"password"}`, admin)
	expectUsersStatus(t, rec, http.StatusCreated)
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	got := decodeJSON[inviteResultBody](t, rec)
	if got.User.Email != "new@example.com" || got.User.Role != serviceuser.RoleMember || got.User.AddedBy != "admin@example.com" {
		t.Fatalf("user = %+v", got.User)
	}
	if got.Method != "password" || got.Delivery != "not_configured" || got.DeliveryError != "" || len(got.TemporaryPassword) < 20 {
		t.Fatalf("result = %+v", got)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, present := raw["deliveryError"]; present {
		t.Fatalf("deliveryError must be omitted when empty: %s", rec.Body.String())
	}
}

func TestPasswordInviteSendEmailDefaultsToTrue(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)

	// No SMTP here: an omitted or true flag asks for mail and finds it unconfigured;
	// only an explicit false reaches not_requested.
	for _, tc := range []struct{ body, delivery string }{
		{`{"email":"a@example.com","role":"member","method":"password"}`, "not_configured"},
		{`{"email":"b@example.com","role":"member","method":"password","sendEmail":true}`, "not_configured"},
		{`{"email":"c@example.com","role":"member","method":"password","sendEmail":false}`, "not_requested"},
	} {
		if got := f.invite(admin, tc.body); got.Delivery != tc.delivery || got.TemporaryPassword == "" {
			t.Fatalf("%s: result = %+v, want delivery %s", tc.body, got, tc.delivery)
		}
	}
}

func TestGoogleInviteKeepsTheLegacyResponse(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)

	for _, body := range []string{
		`{"email":"a@example.com","role":"member"}`,
		`{"email":"b@example.com","role":"member","method":"google"}`,
		`{"email":"c@example.com","role":"member","method":" Google ","sendEmail":false}`,
	} {
		rec := f.do(http.MethodPost, "/api/admin/users", body, admin)
		expectUsersStatus(t, rec, http.StatusCreated)
		if got := rec.Header().Get("Cache-Control"); got == "no-store" {
			t.Fatalf("a Google invite carries no secret, Cache-Control = %q", got)
		}
		var raw map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		if _, ok := raw["email"]; !ok {
			t.Fatalf("%s: want a bare User, got %s", body, rec.Body.String())
		}
		for _, key := range []string{"temporaryPassword", "delivery", "user"} {
			if _, ok := raw[key]; ok {
				t.Fatalf("%s: unexpected %q in %s", body, key, rec.Body.String())
			}
		}
	}
	states := f.auth.PasswordStates(context.Background())
	if len(states) != 0 {
		t.Fatalf("a Google invite created a credential: %+v", states)
	}
}

func TestInviteWithoutMethodChecksGoogleBeforeTheBody(t *testing.T) {
	f := newUsersFixture(t, false)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	const refusal = "configure Google sign-in before adding users"

	// Missing method, explicit google, and an unreadable body all keep the
	// original order: Google first, then the body.
	for _, body := range []string{
		`{"email":"a@example.com","role":"member"}`,
		`{"email":"a@example.com","role":"member","method":"google"}`,
		`{"email":"a@example.com","role":"owner"}`,
		`{`,
		``,
	} {
		rec := f.do(http.MethodPost, "/api/admin/users", body, admin)
		expectUsersStatus(t, rec, http.StatusConflict)
		if !strings.Contains(rec.Body.String(), refusal) {
			t.Fatalf("%q: body = %s", body, rec.Body.String())
		}
	}
	if got, _ := f.users.Get(context.Background(), "a@example.com"); got != nil {
		t.Fatalf("user created: %+v", got)
	}

	// With Google configured the same unreadable body is a plain 400.
	g := newUsersFixture(t, true)
	gAdmin := g.session("admin@example.com", serviceuser.RoleAdmin)
	expectUsersStatus(t, g.do(http.MethodPost, "/api/admin/users", `{`, gAdmin), http.StatusBadRequest)
}

func TestInviteValidation(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	f.invite(admin, `{"email":"taken@example.com","role":"member","method":"password"}`)

	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"unknown method", `{"email":"a@example.com","role":"member","method":"magic"}`, http.StatusBadRequest},
		{"bad role", `{"email":"a@example.com","role":"owner","method":"password"}`, http.StatusBadRequest},
		{"bad email", `{"email":"nope","role":"member","method":"password"}`, http.StatusBadRequest},
		{"bad json", `{"method":`, http.StatusBadRequest},
		{"duplicate password", `{"email":"taken@example.com","role":"member","method":"password"}`, http.StatusConflict},
		{"duplicate google", `{"email":"taken@example.com","role":"member","method":"google"}`, http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := f.do(http.MethodPost, "/api/admin/users", tc.body, admin)
			expectUsersStatus(t, rec, tc.status)
			if strings.Contains(rec.Body.String(), "temporaryPassword") {
				t.Fatalf("error body holds a password field: %s", rec.Body.String())
			}
		})
	}
}

func TestDuplicatePasswordInviteKeepsTheExistingAccount(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	first := f.invite(admin, `{"email":"a@example.com","role":"member","method":"password"}`)

	rec := f.do(http.MethodPost, "/api/admin/users", `{"email":"a@example.com","role":"admin","method":"password"}`, admin)
	expectUsersStatus(t, rec, http.StatusConflict)
	if strings.Contains(rec.Body.String(), first.TemporaryPassword) {
		t.Fatal("a refused invite echoed the existing password")
	}
	got, _ := f.users.Get(context.Background(), "a@example.com")
	if got == nil || got.Role != serviceuser.RoleMember {
		t.Fatalf("existing user changed: %+v", got)
	}
}

func TestAdminRoutesRequireAnAdminOnEveryRoute(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	member := f.session("member@example.com", serviceuser.RoleMember)
	f.invite(admin, `{"email":"target@example.com","role":"member","method":"password"}`)

	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/api/admin/users", ""},
		{http.MethodPost, "/api/admin/users", `{"email":"x@example.com","role":"member","method":"password"}`},
		{http.MethodPost, "/api/admin/users/target@example.com/temporary-password", `{}`},
		{http.MethodPut, "/api/admin/users/target@example.com/role", `{"role":"admin"}`},
		{http.MethodDelete, "/api/admin/users/target@example.com", ""},
	}
	for _, route := range routes {
		expectUsersStatus(t, f.do(route.method, route.path, route.body, nil), http.StatusUnauthorized)
		expectUsersStatus(t, f.do(route.method, route.path, route.body, member), http.StatusForbidden)
	}
	// None of the refused calls had an effect.
	if got, _ := f.users.Get(context.Background(), "target@example.com"); got == nil || got.Role != serviceuser.RoleMember {
		t.Fatalf("target = %+v", got)
	}
	if got, _ := f.users.Get(context.Background(), "x@example.com"); got != nil {
		t.Fatalf("a refused invite created %+v", got)
	}
}

func TestResetIssuesANewTemporaryPassword(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	first := f.invite(admin, `{"email":"member@example.com","role":"member","method":"password"}`)

	for _, body := range []string{``, `{}`, `{"sendEmail":true}`} {
		rec := f.do(http.MethodPost, "/api/admin/users/member@example.com/temporary-password", body, admin)
		expectUsersStatus(t, rec, http.StatusOK)
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control = %q", got)
		}
		got := decodeJSON[inviteResultBody](t, rec)
		if got.User.Email != "member@example.com" || got.Method != "password" || got.Delivery != "not_configured" ||
			got.TemporaryPassword == "" || got.TemporaryPassword == first.TemporaryPassword {
			t.Fatalf("%q: result = %+v", body, got)
		}
	}
	rec := f.do(http.MethodPost, "/api/admin/users/member@example.com/temporary-password", `{"sendEmail":false}`, admin)
	expectUsersStatus(t, rec, http.StatusOK)
	if got := decodeJSON[inviteResultBody](t, rec); got.Delivery != "not_requested" {
		t.Fatalf("delivery = %q", got.Delivery)
	}

	// The percent-encoded form of the address works too.
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users/member%40example.com/temporary-password", `{}`, admin), http.StatusOK)
}

func TestResetGuards(t *testing.T) {
	f := newUsersFixture(t, true)
	const localEmail = "owner@example.com"
	if _, err := f.auth.ClaimLocalAdmin(context.Background(), claimRequest(t, f, localEmail)); err != nil {
		t.Fatal(err)
	}
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)

	rec := f.do(http.MethodPost, "/api/admin/users/missing@example.com/temporary-password", `{}`, admin)
	expectUsersStatus(t, rec, http.StatusNotFound)
	rec = f.do(http.MethodPost, "/api/admin/users/admin@example.com/temporary-password", `{}`, admin)
	expectUsersStatus(t, rec, http.StatusConflict)
	if !strings.Contains(rec.Body.String(), "use Change password") {
		t.Fatalf("self reset body = %s", rec.Body.String())
	}
	rec = f.do(http.MethodPost, "/api/admin/users/owner@example.com/temporary-password", `{}`, admin)
	expectUsersStatus(t, rec, http.StatusConflict)
	if !strings.Contains(rec.Body.String(), "local administrator") {
		t.Fatalf("local admin reset body = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "temporaryPassword") {
		t.Fatal("a refused reset carries a password field")
	}
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users/x@example.com/temporary-password", `{`, admin), http.StatusBadRequest)
	expectUsersStatus(t, f.do(http.MethodGet, "/api/admin/users/x@example.com/temporary-password", "", admin), http.StatusMethodNotAllowed)
	expectUsersStatus(t, f.do(http.MethodPut, "/api/admin/users/x@example.com/temporary-password", "{}", admin), http.StatusMethodNotAllowed)
	expectUsersStatus(t, f.do(http.MethodDelete, "/api/admin/users/x@example.com/temporary-password", "", admin), http.StatusMethodNotAllowed)
	expectUsersStatus(t, f.do(http.MethodDelete, "/api/admin/users/x@example.com/unknown", "", admin), http.StatusNotFound)
}

func TestResetGivesAGoogleOnlyUserAPassword(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users", `{"email":"g@example.com","role":"member"}`, admin), http.StatusCreated)

	before := listUsers(t, f, admin)["g@example.com"]
	if before.PasswordSignIn || before.PasswordChangePending {
		t.Fatalf("before = %+v", before)
	}
	rec := f.do(http.MethodPost, "/api/admin/users/g@example.com/temporary-password", `{}`, admin)
	expectUsersStatus(t, rec, http.StatusOK)
	after := listUsers(t, f, admin)["g@example.com"]
	if !after.PasswordSignIn || !after.PasswordChangePending {
		t.Fatalf("after = %+v", after)
	}
}

type listedUser struct {
	serviceuser.User
	PasswordSignIn        bool `json:"passwordSignIn"`
	PasswordChangePending bool `json:"passwordChangePending"`
}

func listUsers(t *testing.T, f *usersFixture, admin *http.Cookie) map[string]listedUser {
	t.Helper()
	rec := f.do(http.MethodGet, "/api/admin/users", "", admin)
	expectUsersStatus(t, rec, http.StatusOK)
	out := map[string]listedUser{}
	for _, u := range decodeJSON[[]listedUser](t, rec) {
		out[u.Email] = u
	}
	return out
}

func TestUsersListJoinsPasswordState(t *testing.T) {
	f := newUsersFixture(t, true)
	if _, err := f.auth.ClaimLocalAdmin(context.Background(), claimRequest(t, f, "owner@example.com")); err != nil {
		t.Fatal(err)
	}
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	f.invite(admin, `{"email":"pw@example.com","role":"member","method":"password"}`)
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users", `{"email":"g@example.com","role":"member"}`, admin), http.StatusCreated)

	rec := f.do(http.MethodGet, "/api/admin/users", "", admin)
	expectUsersStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"passwordSignIn"`) || !strings.Contains(rec.Body.String(), `"passwordChangePending"`) {
		t.Fatalf("every item must carry both flags: %s", rec.Body.String())
	}
	got := listUsers(t, f, admin)
	if u := got["pw@example.com"]; !u.PasswordSignIn || !u.PasswordChangePending {
		t.Fatalf("invited = %+v", u)
	}
	if u := got["g@example.com"]; u.PasswordSignIn || u.PasswordChangePending {
		t.Fatalf("google = %+v", u)
	}
	if u := got["admin@example.com"]; u.PasswordSignIn || u.PasswordChangePending {
		t.Fatalf("session-only admin = %+v", u)
	}
	if u := got["owner@example.com"]; !u.PasswordSignIn || u.PasswordChangePending {
		t.Fatalf("local admin = %+v", u)
	}
}

func TestRemovalDeletesTheCredentialThroughTheInvitationService(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	first := f.invite(admin, `{"email":"pw@example.com","role":"member","method":"password"}`)

	expectUsersStatus(t, f.do(http.MethodDelete, "/api/admin/users/pw@example.com", "", admin), http.StatusOK)
	expectUsersStatus(t, f.do(http.MethodDelete, "/api/admin/users/pw@example.com", "", admin), http.StatusNotFound)
	if len(f.auth.PasswordStates(context.Background())) != 0 {
		t.Fatal("the credential survived the removal")
	}
	// The old password cannot sign in, even after the address is invited again.
	f.invite(admin, `{"email":"pw@example.com","role":"member","method":"google"}`)
	rec := f.do(http.MethodPost, "/auth/local/login", `{"email":"pw@example.com","password":"`+first.TemporaryPassword+`"}`, nil)
	expectUsersStatus(t, rec, http.StatusUnauthorized)
}

func TestTemporaryPasswordsNeverReachTheLogs(t *testing.T) {
	logs := captureLogs(t)
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)

	first := f.invite(admin, `{"email":"pw@example.com","role":"member","method":"password"}`)
	reset := decodeJSON[inviteResultBody](t, f.do(http.MethodPost, "/api/admin/users/pw@example.com/temporary-password", `{}`, admin))
	// Provoke the failure paths too.
	f.do(http.MethodPost, "/api/admin/users", `{"email":"pw@example.com","role":"member","method":"password"}`, admin)
	f.do(http.MethodPost, "/auth/local/login", `{"email":"pw@example.com","password":"`+reset.TemporaryPassword+`"}`, nil)
	f.do(http.MethodPost, "/auth/local/password/initial", `{"newPassword":"`+first.TemporaryPassword+`"}`, nil)

	for _, secret := range []string{first.TemporaryPassword, reset.TemporaryPassword} {
		if secret == "" {
			t.Fatal("no password was issued")
		}
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("password %q leaked to the log:\n%s", secret, logs.String())
		}
	}
}

func claimRequest(t *testing.T, f *usersFixture, email string) serviceauth.ClaimRequest {
	t.Helper()
	return serviceauth.ClaimRequest{Email: email, Password: "local admin password", SetupToken: setupTokenFor(t, f.auth)}
}
