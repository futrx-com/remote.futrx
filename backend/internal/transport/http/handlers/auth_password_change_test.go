package httphandlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	serviceapplications "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileapplications"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
)

const newTestPassword = "a brand new passphrase"

func (f *usersFixture) post(path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

// postFrom is post from a given client address, so cases that burn the
// per-address budget do not starve each other.
func (f *usersFixture) postFrom(ip, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("X-Forwarded-For", ip)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func responseCookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// invitedLogin invites email with a password, signs in with the temporary
// password and returns it with the password-change cookie the browser holds.
func (f *usersFixture) invitedLogin(admin *http.Cookie, email string, role serviceuser.Role) (string, *http.Cookie) {
	f.t.Helper()
	invited := f.invite(admin, `{"email":"`+email+`","role":"`+string(role)+`","method":"password"}`)
	return invited.TemporaryPassword, f.loginForChangeCookie(email, invited.TemporaryPassword)
}

func (f *usersFixture) loginForChangeCookie(email, password string) *http.Cookie {
	f.t.Helper()
	rec := f.post("/auth/local/login", `{"email":"`+email+`","password":"`+password+`"}`)
	expectUsersStatus(f.t, rec, http.StatusOK)
	cookie := responseCookieNamed(rec, passwordChangeCookieName)
	if cookie == nil {
		f.t.Fatalf("no password-change cookie: %s", rec.Body.String())
	}
	return &http.Cookie{Name: cookie.Name, Value: cookie.Value}
}

func TestTemporaryPasswordLoginSetsOnlyAHostScopedChangeCookie(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	invited := f.invite(admin, `{"email":"member@example.com","role":"member","method":"password"}`)

	rec := f.post("/auth/local/login", `{"email":"Member@Example.com","password":"`+invited.TemporaryPassword+`"}`)
	expectUsersStatus(t, rec, http.StatusOK)
	if got := strings.TrimSpace(rec.Body.String()); got != `{"passwordChangeRequired":true}` {
		t.Fatalf("body = %s", got)
	}
	if session := responseCookieNamed(rec, serviceauth.SessionCookieName); session != nil {
		t.Fatalf("a session was issued before the change: %+v", session)
	}
	if pending := responseCookieNamed(rec, serviceauth.PendingTwoFactorCookieName); pending != nil {
		t.Fatalf("unexpected 2FA cookie: %+v", pending)
	}
	cookie := responseCookieNamed(rec, passwordChangeCookieName)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("no password-change cookie")
	}
	if cookie.Name != "remote_password_change" {
		t.Fatalf("name = %q", cookie.Name)
	}
	if cookie.Domain != "" {
		t.Fatalf("Domain = %q, want host-only", cookie.Domain)
	}
	if cookie.Path != "/auth/local/password" {
		t.Fatalf("Path = %q", cookie.Path)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("flags = %+v", cookie)
	}
	if cookie.MaxAge != int((15 * time.Minute).Seconds()) {
		t.Fatalf("MaxAge = %d", cookie.MaxAge)
	}
	for _, header := range rec.Result().Header.Values("Set-Cookie") {
		if strings.HasPrefix(header, passwordChangeCookieName+"=") && strings.Contains(strings.ToLower(header), "domain") {
			t.Fatalf("Set-Cookie carries a Domain: %s", header)
		}
	}

	// A wrong password is still the same 401 as before.
	rec = f.post("/auth/local/login", `{"email":"member@example.com","password":"not the password"}`)
	expectUsersStatus(t, rec, http.StatusUnauthorized)
	if responseCookieNamed(rec, passwordChangeCookieName) != nil {
		t.Fatal("a wrong password received a change cookie")
	}
}

func TestLocalAdminLoginIsUnchanged(t *testing.T) {
	f := newUsersFixture(t, true)
	claim := claimRequest(t, f, "owner@example.com")
	if _, err := f.auth.ClaimLocalAdmin(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	rec := f.post("/auth/local/login", `{"email":"owner@example.com","password":"`+claim.Password+`"}`)
	expectUsersStatus(t, rec, http.StatusOK)
	if responseCookieNamed(rec, serviceauth.SessionCookieName) == nil || responseCookieNamed(rec, passwordChangeCookieName) != nil {
		t.Fatalf("cookies = %+v", rec.Result().Cookies())
	}
	status := decodeJSON[serviceauth.Status](t, rec)
	if !status.Authenticated || !status.IsAdmin {
		t.Fatalf("status = %+v", status)
	}
}

// gatewayHandler is the application-origin handler of the platform, built on
// the fixture's own auth service.
func (f *usersFixture) gatewayHandler() http.Handler {
	f.t.Helper()
	registry := &webTestRegistry{application: serviceapplications.Application{
		ID: "editor", Scopes: []serviceapplications.Scope{serviceapplications.ScopeProject},
		Web: &serviceapplications.ApplicationWeb{Port: 8400, Subdomain: "editor"},
	}}
	store, err := fileapplications.New(f.t.TempDir())
	if err != nil {
		f.t.Fatal(err)
	}
	instance := serviceapplications.Instance{ID: webTestID, ApplicationID: "editor", Scope: serviceapplications.ScopeProject, ProjectID: "aaaa1111", Status: serviceapplications.StatusRunning}
	if err := store.Put(context.Background(), instance); err != nil {
		f.t.Fatal(err)
	}
	projects := webTestProjects{project: serviceproject.Meta{ID: "aaaa1111", Slug: "project"}}
	apps := NewApplicationsHandler(serviceapplications.New(registry, store, nil, nil, nil), f.auth, projects).WithWebHost("remote.example.com")
	platform := http.NewServeMux()
	apps.RegisterRoutes(platform)
	return httpmiddleware.NewBrowserProtection(f.auth.BaseURL()).Wrap(apps.WebHandler(platform))
}

func TestChangeCookieAloneOpensNothing(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	_, change := f.invitedLogin(admin, "member@example.com", serviceuser.RoleMember)
	// Whatever a browser would send, the platform must not honour it: the
	// cookie is attached by hand to paths it would never be sent to.
	guarded := httpmiddleware.NewAuth(f.auth).Wrap(f.mux)

	for _, path := range []string{"/api/projects", "/api/admin/users", "/api/me/security", "/ws/terminal", "/ws"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(change)
		rec := httptest.NewRecorder()
		guarded.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: status = %d, want 401", path, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(change)
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)
	if status := decodeJSON[serviceauth.Status](t, rec); status.Authenticated || status.Email != "" {
		t.Fatalf("/auth/me with only the change cookie = %+v", status)
	}

	verify := &authVerifyHandler{
		auth:   f.auth,
		access: serviceauth.NewAccessVerifier(f.auth, verifyProjectDirectory{}),
		shares: &shareAuthorizerStub{},
	}
	rec = verifyRequest(t, verify, verifyPreviewHost, "/", change)
	if rec.Code == http.StatusOK {
		t.Fatal("/auth/verify accepted the change cookie")
	}
	rec = verifyRequest(t, &authVerifyHandler{auth: f.auth, access: verify.access, shares: &shareAuthorizerStub{}}, "alpha--3000.dev.remote.example.com", "/", change)
	if rec.Code == http.StatusOK {
		t.Fatal("/auth/verify accepted the change cookie on the preview host")
	}

	gateway := f.gatewayHandler()
	host := "https://editor--project.remote.example.com/"
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req := httptest.NewRequest(method, host, nil)
		req.AddCookie(change)
		rec := httptest.NewRecorder()
		gateway.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound && rec.Code != http.StatusUnauthorized {
			t.Fatalf("gateway %s: status = %d, want a login redirect or 401", method, rec.Code)
		}
		if rec.Code == http.StatusFound && !strings.HasPrefix(rec.Header().Get("Location"), "https://remote.example.com/?return_to=") {
			t.Fatalf("gateway %s: Location = %q", method, rec.Header().Get("Location"))
		}
	}
}

func TestInitialPasswordCompletesTheSignIn(t *testing.T) {
	for _, tc := range []struct {
		role    serviceuser.Role
		isAdmin bool
	}{{serviceuser.RoleMember, false}, {serviceuser.RoleAdmin, true}} {
		t.Run(string(tc.role), func(t *testing.T) {
			f := newUsersFixture(t, true)
			admin := f.session("admin@example.com", serviceuser.RoleAdmin)
			temp, change := f.invitedLogin(admin, "user@example.com", tc.role)

			// Reachable with no session, through the same middleware the server uses.
			guarded := httpmiddleware.NewAuth(f.auth).Wrap(f.mux)
			req := httptest.NewRequest(http.MethodPost, "/auth/local/password/initial", strings.NewReader(`{"newPassword":"`+newTestPassword+`"}`))
			req.AddCookie(change)
			rec := httptest.NewRecorder()
			guarded.ServeHTTP(rec, req)
			expectUsersStatus(t, rec, http.StatusOK)

			status := decodeJSON[serviceauth.Status](t, rec)
			if !status.Authenticated || status.Email != "user@example.com" || status.IsAdmin != tc.isAdmin {
				t.Fatalf("status = %+v", status)
			}
			session := responseCookieNamed(rec, serviceauth.SessionCookieName)
			if session == nil || session.Value == "" || session.MaxAge <= 0 {
				t.Fatalf("session cookie = %+v", session)
			}
			cleared := responseCookieNamed(rec, passwordChangeCookieName)
			if cleared == nil || cleared.MaxAge >= 0 || cleared.Path != "/auth/local/password" || cleared.Domain != "" {
				t.Fatalf("change cookie not cleared host-only: %+v", cleared)
			}

			// /auth/me shows the same rights, and an admin route follows the role.
			me := f.do(http.MethodGet, "/auth/me", "", &http.Cookie{Name: session.Name, Value: session.Value})
			meStatus := decodeJSON[serviceauth.Status](t, me)
			if !meStatus.Authenticated || meStatus.IsAdmin != tc.isAdmin || meStatus.Email != "user@example.com" {
				t.Fatalf("/auth/me = %+v", meStatus)
			}
			wantAdminRoute := http.StatusForbidden
			if tc.isAdmin {
				wantAdminRoute = http.StatusOK
			}
			expectUsersStatus(t, f.do(http.MethodGet, "/api/admin/users", "", &http.Cookie{Name: session.Name, Value: session.Value}), wantAdminRoute)

			// The temporary password is dead and the new one signs in directly.
			expectUsersStatus(t, f.post("/auth/local/login", `{"email":"user@example.com","password":"`+temp+`"}`), http.StatusUnauthorized)
			rec = f.post("/auth/local/login", `{"email":"user@example.com","password":"`+newTestPassword+`"}`)
			expectUsersStatus(t, rec, http.StatusOK)
			if responseCookieNamed(rec, serviceauth.SessionCookieName) == nil || responseCookieNamed(rec, passwordChangeCookieName) != nil {
				t.Fatalf("cookies = %+v", rec.Result().Cookies())
			}

			// The change token is single-use.
			rec = f.post("/auth/local/password/initial", `{"newPassword":"another passphrase!"}`, change)
			expectUsersStatus(t, rec, http.StatusBadRequest)
			if list := listUsers(t, f, admin)["user@example.com"]; list.PasswordChangePending || !list.PasswordSignIn {
				t.Fatalf("list state = %+v", list)
			}
		})
	}
}

func TestInitialPasswordRejectionsKeepTheCookie(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	temp, change := f.invitedLogin(admin, "member@example.com", serviceuser.RoleMember)

	for _, tc := range []struct{ name, password string }{
		{"too short", "short"},
		{"empty", ""},
		{"same as the temporary one", temp},
		{"too long", strings.Repeat("x", 1025)},
	} {
		rec := f.post("/auth/local/password/initial", `{"newPassword":"`+tc.password+`"}`, change)
		expectUsersStatus(t, rec, http.StatusBadRequest)
		if cleared := responseCookieNamed(rec, passwordChangeCookieName); cleared != nil {
			t.Fatalf("%s: the cookie was touched: %+v", tc.name, cleared)
		}
		if responseCookieNamed(rec, serviceauth.SessionCookieName) != nil {
			t.Fatalf("%s: a session was issued", tc.name)
		}
		if strings.Contains(rec.Body.String(), temp) {
			t.Fatalf("%s: the body echoes the temporary password", tc.name)
		}
	}
	// Malformed body: 400 too, cookie kept.
	rec := f.post("/auth/local/password/initial", `{`, change)
	expectUsersStatus(t, rec, http.StatusBadRequest)
	if responseCookieNamed(rec, passwordChangeCookieName) != nil {
		t.Fatal("a malformed body cleared the cookie")
	}
	// The same cookie still completes the change.
	expectUsersStatus(t, f.post("/auth/local/password/initial", `{"newPassword":"`+newTestPassword+`"}`, change), http.StatusOK)
}

// forgedChangeToken signs a payload the way the service does, to reach the
// expired and wrong-purpose cases that real time cannot.
func forgedChangeToken(domain string, payload map[string]any) string {
	body, _ := json.Marshal(payload)
	b64 := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte("test-session-key"))
	if domain != "" {
		mac.Write([]byte(domain))
		mac.Write([]byte{0})
	}
	mac.Write([]byte(b64))
	return b64 + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestInitialPasswordRefusesInvalidTokensWith400(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	_, change := f.invitedLogin(admin, "member@example.com", serviceuser.RoleMember)

	payload := func(exp int64) map[string]any {
		return map[string]any{"email": "member@example.com", "sub": "password", "method": "password", "version": 1, "fp": "x", "exp": exp}
	}
	sessionValue := f.session2("other@example.com")
	last := len(change.Value) - 1
	flipped := change.Value[:last] + map[bool]string{true: "B", false: "A"}[change.Value[last] == 'A']

	for i, tc := range []struct {
		name   string
		cookie *http.Cookie
	}{
		{"missing", nil},
		{"empty", &http.Cookie{Name: passwordChangeCookieName, Value: ""}},
		{"garbage", &http.Cookie{Name: passwordChangeCookieName, Value: "not-a-token"}},
		{"tampered signature", &http.Cookie{Name: passwordChangeCookieName, Value: flipped}},
		{"tampered payload", &http.Cookie{Name: passwordChangeCookieName, Value: "e30" + change.Value[strings.Index(change.Value, "."):]}},
		{"expired", &http.Cookie{Name: passwordChangeCookieName, Value: forgedChangeToken("password-change/v1", payload(time.Now().Add(-time.Minute).Unix()))}},
		{"signed for another purpose", &http.Cookie{Name: passwordChangeCookieName, Value: forgedChangeToken("pending-login/v1", payload(time.Now().Add(time.Hour).Unix()))}},
		{"a session in its place", &http.Cookie{Name: passwordChangeCookieName, Value: sessionValue}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cookies []*http.Cookie
			if tc.cookie != nil {
				cookies = append(cookies, tc.cookie)
			}
			rec := f.postFrom(fmt.Sprintf("198.51.100.%d", i+1), "/auth/local/password/initial", `{"newPassword":"`+newTestPassword+`"}`, cookies...)
			expectUsersStatus(t, rec, http.StatusBadRequest)
			if !strings.Contains(rec.Body.String(), "this sign-in step expired, sign in again") {
				t.Fatalf("body = %s", rec.Body.String())
			}
			cleared := responseCookieNamed(rec, passwordChangeCookieName)
			if cleared == nil || cleared.MaxAge >= 0 || cleared.Path != "/auth/local/password" {
				t.Fatalf("the cookie was not cleared: %+v", cleared)
			}
			if responseCookieNamed(rec, serviceauth.SessionCookieName) != nil {
				t.Fatal("a session was issued")
			}
		})
	}
	// The genuine cookie is unaffected by all of that.
	expectUsersStatus(t, f.post("/auth/local/password/initial", `{"newPassword":"`+newTestPassword+`"}`, change), http.StatusOK)
}

func (f *usersFixture) session2(email string) string {
	f.t.Helper()
	return f.session(email, serviceuser.RoleMember).Value
}

func TestResetInvalidatesAnOpenPasswordChange(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	temp, change := f.invitedLogin(admin, "member@example.com", serviceuser.RoleMember)

	reset := decodeJSON[inviteResultBody](t, f.do(http.MethodPost, "/api/admin/users/member@example.com/temporary-password", `{}`, admin))

	rec := f.post("/auth/local/password/initial", `{"newPassword":"`+newTestPassword+`"}`, change)
	expectUsersStatus(t, rec, http.StatusBadRequest)
	if cleared := responseCookieNamed(rec, passwordChangeCookieName); cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("not cleared: %+v", cleared)
	}
	expectUsersStatus(t, f.post("/auth/local/login", `{"email":"member@example.com","password":"`+temp+`"}`), http.StatusUnauthorized)
	// The new temporary password leads to a fresh change step.
	fresh := f.loginForChangeCookie("member@example.com", reset.TemporaryPassword)
	expectUsersStatus(t, f.post("/auth/local/password/initial", `{"newPassword":"`+newTestPassword+`"}`, fresh), http.StatusOK)
}

func TestInitialPasswordIsRateLimited(t *testing.T) {
	f := newUsersFixture(t, true)
	for i := 0; i < 5; i++ {
		expectUsersStatus(t, f.post("/auth/local/password/initial", `{"newPassword":"`+newTestPassword+`"}`), http.StatusBadRequest)
	}
	rec := f.post("/auth/local/password/initial", `{"newPassword":"`+newTestPassword+`"}`)
	expectUsersStatus(t, rec, http.StatusTooManyRequests)
	if rec.Header().Get("Retry-After") != "300" {
		t.Fatalf("Retry-After = %q", rec.Header().Get("Retry-After"))
	}

	// A different client address has its own budget, and the limit does not
	// spill over to the regular login.
	req := httptest.NewRequest(http.MethodPost, "/auth/local/password/initial", strings.NewReader(`{"newPassword":"`+newTestPassword+`"}`))
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	other := httptest.NewRecorder()
	f.mux.ServeHTTP(other, req)
	expectUsersStatus(t, other, http.StatusBadRequest)
	expectUsersStatus(t, f.post("/auth/local/login", `{"email":"x@example.com","password":"nope"}`), http.StatusUnauthorized)
}

func TestRejectedPasswordsDoNotConsumeTheRateLimit(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	_, change := f.invitedLogin(admin, "member@example.com", serviceuser.RoleMember)
	for i := 0; i < 8; i++ {
		expectUsersStatus(t, f.post("/auth/local/password/initial", `{"newPassword":"short"}`, change), http.StatusBadRequest)
	}
	expectUsersStatus(t, f.post("/auth/local/password/initial", `{"newPassword":"`+newTestPassword+`"}`, change), http.StatusOK)
}

func TestPasswordChangeEndpointsRejectOtherMethods(t *testing.T) {
	f := newUsersFixture(t, true)
	for _, path := range []string{"/auth/local/password/initial", "/auth/local/password/cancel"} {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			req := httptest.NewRequest(method, path, nil)
			rec := httptest.NewRecorder()
			f.mux.ServeHTTP(rec, req)
			expectUsersStatus(t, rec, http.StatusMethodNotAllowed)
		}
	}
}

func TestCancelClearsTheChangeCookie(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	_, change := f.invitedLogin(admin, "member@example.com", serviceuser.RoleMember)

	rec := f.post("/auth/local/password/cancel", "", change)
	expectUsersStatus(t, rec, http.StatusNoContent)
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q", rec.Body.String())
	}
	cleared := responseCookieNamed(rec, passwordChangeCookieName)
	if cleared == nil || cleared.MaxAge >= 0 || cleared.Path != "/auth/local/password" || cleared.Domain != "" ||
		!cleared.HttpOnly || !cleared.Secure || cleared.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cleared = %+v", cleared)
	}
	// Cancelling without a cookie is fine too, and needs no session.
	expectUsersStatus(t, f.post("/auth/local/password/cancel", ""), http.StatusNoContent)
	// Cancelling does not revoke the temporary password: signing in again works.
	if f.loginForChangeCookie("member@example.com", f.tempFor("member@example.com", admin)) == nil {
		t.Fatal("no cookie")
	}
}

// tempFor resets email and returns the new temporary password.
func (f *usersFixture) tempFor(email string, admin *http.Cookie) string {
	f.t.Helper()
	rec := f.do(http.MethodPost, "/api/admin/users/"+email+"/temporary-password", `{}`, admin)
	expectUsersStatus(f.t, rec, http.StatusOK)
	return decodeJSON[inviteResultBody](f.t, rec).TemporaryPassword
}

// googleCallback drives /auth/google/callback as the given Google user.
func (f *usersFixture) googleCallback(user serviceauth.User, extra ...*http.Cookie) *httptest.ResponseRecorder {
	f.t.Helper()
	f.oauth.user = user
	loginRec := httptest.NewRecorder()
	f.mux.ServeHTTP(loginRec, httptest.NewRequest(http.MethodGet, "/auth/google/login", nil))
	state := responseCookieNamed(loginRec, serviceauth.StateCookieName)
	if state == nil {
		f.t.Fatalf("no state cookie: %d", loginRec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=abc&state="+url.QueryEscape(state.Value), nil)
	req.AddCookie(&http.Cookie{Name: state.Name, Value: state.Value})
	for _, c := range extra {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func TestGoogleSignInOfAPendingAccountRedirectsToTheChangeStep(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	f.invite(admin, `{"email":"member@example.com","role":"member","method":"password"}`)

	rec := f.googleCallback(serviceauth.User{Email: "member@example.com", Sub: "google-123"})
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "https://remote.example.com/?passwordChangeRequired=1" {
		t.Fatalf("Location = %q", got)
	}
	if responseCookieNamed(rec, serviceauth.SessionCookieName) != nil {
		t.Fatal("a session was issued before the change")
	}
	cookie := responseCookieNamed(rec, passwordChangeCookieName)
	if cookie == nil || cookie.Domain != "" || cookie.Path != "/auth/local/password" || !cookie.HttpOnly || !cookie.Secure ||
		cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != 900 {
		t.Fatalf("cookie = %+v", cookie)
	}

	// The same cookie finishes the sign-in with the Google subject.
	rec = f.post("/auth/local/password/initial", `{"newPassword":"`+newTestPassword+`"}`, &http.Cookie{Name: cookie.Name, Value: cookie.Value})
	expectUsersStatus(t, rec, http.StatusOK)
	status := decodeJSON[serviceauth.Status](t, rec)
	if !status.Authenticated || status.Email != "member@example.com" || status.Sub != "google-123" {
		t.Fatalf("status = %+v", status)
	}

	// Once changed, Google signs straight in again.
	rec = f.googleCallback(serviceauth.User{Email: "member@example.com", Sub: "google-123"})
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" || responseCookieNamed(rec, serviceauth.SessionCookieName) == nil ||
		responseCookieNamed(rec, passwordChangeCookieName) != nil {
		t.Fatalf("status = %d Location = %q cookies = %+v", rec.Code, rec.Header().Get("Location"), rec.Result().Cookies())
	}
}

func TestGoogleOnlyUserResetMustChangeOnTheNextGoogleSignIn(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users", `{"email":"g@example.com","role":"member"}`, admin), http.StatusCreated)

	rec := f.googleCallback(serviceauth.User{Email: "g@example.com", Sub: "google-9"})
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" || responseCookieNamed(rec, serviceauth.SessionCookieName) == nil {
		t.Fatalf("before the reset: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	f.tempFor("g@example.com", admin)
	rec = f.googleCallback(serviceauth.User{Email: "g@example.com", Sub: "google-9"})
	if rec.Header().Get("Location") != "https://remote.example.com/?passwordChangeRequired=1" || responseCookieNamed(rec, serviceauth.SessionCookieName) != nil {
		t.Fatalf("after the reset: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestInitialPasswordHandsOverToTwoFactor(t *testing.T) {
	f := newUsersFixture(t, true)
	admin := f.session("admin@example.com", serviceuser.RoleAdmin)
	expectUsersStatus(t, f.do(http.MethodPost, "/api/admin/users", `{"email":"g@example.com","role":"member"}`, admin), http.StatusCreated)

	// The Google-only user turns two-factor on while they still have a session.
	value, err := f.auth.IssueSession(context.Background(), serviceauth.User{Email: "g@example.com", Sub: "google-9"}, serviceauth.SignInMethodGoogle, "", "")
	if err != nil {
		t.Fatal(err)
	}
	owner := &http.Cookie{Name: serviceauth.SessionCookieName, Value: value}
	enroll := f.post("/api/me/security/2fa/enroll", "", owner)
	expectUsersStatus(t, enroll, http.StatusOK)
	enrolled := decodeJSON[struct {
		EnrollmentToken string `json:"enrollmentToken"`
		Secret          string `json:"secret"`
	}](t, enroll)
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrolled.Secret)
	if err != nil {
		t.Fatal(err)
	}
	confirm := f.post("/api/me/security/2fa/confirm", `{"enrollmentToken":"`+enrolled.EnrollmentToken+`","code":"`+serviceauth.TOTPCode(secret, time.Now())+`"}`, owner)
	expectUsersStatus(t, confirm, http.StatusOK)

	// An administrator resets them; they sign in with the temporary password.
	temp := f.tempFor("g@example.com", admin)
	change := f.loginForChangeCookie("g@example.com", temp)

	rec := f.post("/auth/local/password/initial", `{"newPassword":"`+newTestPassword+`"}`, change)
	expectUsersStatus(t, rec, http.StatusOK)
	if got := strings.TrimSpace(rec.Body.String()); got != `{"twoFactorRequired":true}` {
		t.Fatalf("body = %s", got)
	}
	if responseCookieNamed(rec, serviceauth.SessionCookieName) != nil {
		t.Fatal("a session was issued before the second factor")
	}
	pending := responseCookieNamed(rec, serviceauth.PendingTwoFactorCookieName)
	if pending == nil || pending.Value == "" || pending.MaxAge <= 0 {
		t.Fatalf("pending cookie = %+v", pending)
	}
	if cleared := responseCookieNamed(rec, passwordChangeCookieName); cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("change cookie not cleared: %+v", cleared)
	}

	// The second factor then yields the session.
	verify := f.post("/auth/2fa/verify", `{"code":"`+serviceauth.TOTPCode(secret, time.Now().Add(30*time.Second))+`"}`, &http.Cookie{Name: pending.Name, Value: pending.Value})
	expectUsersStatus(t, verify, http.StatusOK)
	if !decodeJSON[serviceauth.Status](t, verify).Authenticated || responseCookieNamed(verify, serviceauth.SessionCookieName) == nil {
		t.Fatalf("verify = %s", verify.Body.String())
	}
}
