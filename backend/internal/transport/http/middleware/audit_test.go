package httpmiddleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
)

type auditRoleDirectory struct {
	admins map[string]bool
}

func (d auditRoleDirectory) IsAdmin(_ context.Context, email string) (bool, error) {
	return d.admins[email], nil
}

func (d auditRoleDirectory) IsRegistered(_ context.Context, email string) (bool, error) {
	_, ok := d.admins[email]
	return ok, nil
}

func (auditRoleDirectory) AddBootstrapAdmin(context.Context, string) error { return nil }

func (auditRoleDirectory) FirstAdmin(context.Context) (*serviceauth.UserDirectoryEntry, error) {
	return &serviceauth.UserDirectoryEntry{Email: "admin@example.com"}, nil
}

// auditCallerProbe captures whatever caller the middleware left on the request
// context, which is the contract every instrumented service depends on.
func auditCallerProbe(seen *serviceaudit.Caller, found *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen, *found = serviceaudit.CallerFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
}

func newAuditMiddleware(t *testing.T, admins map[string]bool) (*serviceauth.Service, *Auth) {
	t.Helper()
	auth, err := serviceauth.New(
		context.Background(),
		fileauth.New(t.TempDir()),
		auditRoleDirectory{admins: admins},
		func(string, string, string) serviceauth.OAuthProvider { return authTestOAuth{} },
		"https://remote.example.com",
		[]byte("test-session-key"),
	)
	if err != nil {
		t.Fatalf("New auth service: %v", err)
	}
	middleware := NewAuth(auth).
		RequireLocalAdminSetup(func() bool { return true }).
		RequireProviderLogin(func() bool { return true })
	return auth, middleware
}

func TestMiddlewarePropagatesTheAuditActor(t *testing.T) {
	admins := map[string]bool{"admin@example.com": true, "member@example.com": false}
	auth, middleware := newAuditMiddleware(t, admins)

	tests := []struct {
		name        string
		email       string
		wantIsAdmin bool
	}{
		{name: "admin", email: "admin@example.com", wantIsAdmin: true},
		{name: "member", email: "member@example.com", wantIsAdmin: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seen serviceaudit.Caller
			found := false
			handler := middleware.Wrap(auditCallerProbe(&seen, &found))

			req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
			req.AddCookie(&http.Cookie{
				Name:  serviceauth.SessionCookieName,
				Value: auth.SignSession(serviceauth.User{Email: tc.email, Sub: "sub-1"}),
			})
			req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
			req.Header.Set("User-Agent", "Mozilla/5.0 (test)")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusNoContent, rec.Body.String())
			}
			if !found {
				t.Fatal("no audit caller on the request context")
			}
			if seen.Actor.Email != tc.email || seen.Actor.Sub != "sub-1" {
				t.Fatalf("actor = %+v, want the session principal", seen.Actor)
			}
			if seen.Actor.IsAdmin != tc.wantIsAdmin {
				t.Fatalf("isAdmin = %t, want %t", seen.Actor.IsAdmin, tc.wantIsAdmin)
			}
			if seen.IP != "203.0.113.5" {
				t.Fatalf("ip = %q, want the left-most forwarded address", seen.IP)
			}
			if seen.UserAgent != "Mozilla/5.0 (test)" {
				t.Fatalf("userAgent = %q", seen.UserAgent)
			}
		})
	}
}

func TestMiddlewareGivesAuthRoutesTheRequestHalfOnly(t *testing.T) {
	_, middleware := newAuditMiddleware(t, map[string]bool{})

	var seen serviceaudit.Caller
	found := false
	handler := middleware.Wrap(auditCallerProbe(&seen, &found))

	req := httptest.NewRequest(http.MethodPost, "/auth/local/login", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.4")
	req.Header.Set("User-Agent", "curl/8.7.1")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !found {
		t.Fatal("sign-in requests need a caller so the login handler can stamp IP and user agent")
	}
	if !seen.Actor.Empty() {
		t.Fatalf("actor = %+v, want it unresolved before the credentials are checked", seen.Actor)
	}
	if seen.IP != "198.51.100.4" || seen.UserAgent != "curl/8.7.1" {
		t.Fatalf("request fields = %q/%q", seen.IP, seen.UserAgent)
	}
}

func TestMiddlewareLeavesStaticRequestsUnstamped(t *testing.T) {
	_, middleware := newAuditMiddleware(t, map[string]bool{})

	var seen serviceaudit.Caller
	found := false
	handler := middleware.Wrap(auditCallerProbe(&seen, &found))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/index.html", nil))

	if found {
		t.Fatal("a static asset request carried an audit caller")
	}
}
