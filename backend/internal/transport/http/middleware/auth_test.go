package httpmiddleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
)

type authTestDirectory struct{}

func (authTestDirectory) IsAdmin(context.Context, string) (bool, error)      { return true, nil }
func (authTestDirectory) IsRegistered(context.Context, string) (bool, error) { return true, nil }
func (authTestDirectory) AddBootstrapAdmin(context.Context, string) error    { return nil }
func (authTestDirectory) FirstAdmin(context.Context) (*serviceauth.UserDirectoryEntry, error) {
	return &serviceauth.UserDirectoryEntry{Email: "admin@example.com"}, nil
}

type authTestOAuth struct{}

func (authTestOAuth) AuthCodeURL(string) string { return "https://accounts.example.com" }
func (authTestOAuth) ExchangeUser(context.Context, string) (serviceauth.User, error) {
	return serviceauth.User{}, nil
}

func TestProviderLoginGate(t *testing.T) {
	auth, err := serviceauth.New(
		context.Background(),
		fileauth.New(t.TempDir()),
		authTestDirectory{},
		func(string, string, string) serviceauth.OAuthProvider { return authTestOAuth{} },
		"https://remote.example.com",
		[]byte("test-session-key"),
	)
	if err != nil {
		t.Fatalf("New auth service: %v", err)
	}

	providerReady := false
	localAdminReady := true
	handler := NewAuth(auth).
		RequireLocalAdminSetup(func() bool { return localAdminReady }).
		RequireProviderLogin(
			func() bool { return providerReady },
			"/api/claude/",
			"/ws/claude/auth-status",
			"/api/agent-auth",
			"/ws/agent-auth/",
		).
		Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
	session := auth.SignSession(serviceauth.User{Email: "admin@example.com", Sub: "admin"})

	request := func(path string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: session})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res.Code
	}

	if got := request("/api/projects"); got != http.StatusPreconditionRequired {
		t.Fatalf("workspace status = %d, want %d", got, http.StatusPreconditionRequired)
	}
	if got := request("/api/claude/auth-status"); got != http.StatusNoContent {
		t.Fatalf("provider auth status = %d, want %d", got, http.StatusNoContent)
	}
	if got := request("/api/agent-auth"); got != http.StatusNoContent {
		t.Fatalf("agent auth catalog = %d, want %d", got, http.StatusNoContent)
	}
	if got := request("/ws/agent-auth/future-agent"); got != http.StatusNoContent {
		t.Fatalf("normalized provider stream = %d, want %d", got, http.StatusNoContent)
	}
	localAdminReady = false
	if got := request("/api/claude/auth-status"); got != http.StatusPreconditionRequired {
		t.Fatalf("provider status before local admin setup = %d, want %d", got, http.StatusPreconditionRequired)
	}
	localAdminReady = true
	providerReady = true
	if got := request("/api/projects"); got != http.StatusNoContent {
		t.Fatalf("authenticated workspace status = %d, want %d", got, http.StatusNoContent)
	}
}
