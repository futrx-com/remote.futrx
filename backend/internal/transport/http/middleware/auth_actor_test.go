package httpmiddleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	"github.com/futrx-com/remote.futrx.com/internal/service/permission"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filesessions"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filetwofactor"
)

// actorTestDirectory registers only member@example.com, matching case- and
// whitespace-insensitively like the real user directory.
type actorTestDirectory struct{ authTestDirectory }

func (actorTestDirectory) IsAdmin(context.Context, string) (bool, error) { return false, nil }
func (actorTestDirectory) IsRegistered(_ context.Context, email string) (bool, error) {
	return strings.ToLower(strings.TrimSpace(email)) == "member@example.com", nil
}

func newActorTestAuth(t *testing.T) *serviceauth.Service {
	t.Helper()
	twoFactorStore, err := filetwofactor.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sessionRegistryStore, err := filesessions.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	auth, err := serviceauth.New(
		context.Background(),
		fileauth.New(t.TempDir()),
		actorTestDirectory{},
		func(string, string, string) serviceauth.OAuthProvider { return authTestOAuth{} },
		"https://remote.example.com",
		[]byte("test-session-key"),
		twoFactorStore,
		sessionRegistryStore,
		serviceauth.Options{
			PendingLoginTTL:     5 * time.Minute,
			EnrollmentTTL:       10 * time.Minute,
			RecoveryCodeCount:   10,
			SessionHistoryLimit: 20,
			SetupTokenTTL:       30 * time.Minute,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func TestWrapAttachesTheAuthenticatedActorAfterRegistrationIsVerified(t *testing.T) {
	auth := newActorTestAuth(t)
	var got permission.Actor
	var found bool
	handler := NewAuth(auth).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, found = permission.ActorFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	session, err := auth.IssueSession(
		context.Background(), serviceauth.User{Email: "Member@Example.com", Sub: "member"},
		serviceauth.SignInMethodGoogle, "", "",
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/api/projects", "/ws/terminal"} {
		found = false
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: session})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want %d", path, response.Code, http.StatusNoContent)
		}
		if !found || got.Email != "member@example.com" || got.IsSystem() {
			t.Fatalf("%s actor = %+v (found %v), want the normalized member", path, got, found)
		}
	}
}

func TestWrapNeverForwardsAnActorForRejectedOrUnauthenticatedRequests(t *testing.T) {
	auth := newActorTestAuth(t)
	reached := false
	handler := NewAuth(auth).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if _, ok := permission.ActorFromContext(r.Context()); ok && r.URL.Path != "/" {
			t.Errorf("%s: an actor was attached to an unauthenticated request", r.URL.Path)
		}
	}))
	unregistered, err := auth.IssueSession(
		context.Background(), serviceauth.User{Email: "stranger@example.com", Sub: "stranger"},
		serviceauth.SignInMethodGoogle, "", "",
	)
	if err != nil {
		t.Fatal(err)
	}

	anonymous := httptest.NewRecorder()
	handler.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/projects", nil))
	if anonymous.Code != http.StatusUnauthorized || reached {
		t.Fatalf("anonymous: status = %d, reached = %v", anonymous.Code, reached)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	request.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: unregistered})
	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, request)
	if rejected.Code != http.StatusUnauthorized || reached {
		t.Fatalf("unregistered: status = %d, reached = %v", rejected.Code, reached)
	}

	// Static assets and auth routes pass through without an actor.
	reached = false
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/auth/me", nil))
	if !reached {
		t.Fatal("/auth/me was not passed through")
	}
}

func TestWrapDoesNotTrustAnActorAlreadyInTheRequestContext(t *testing.T) {
	auth := newActorTestAuth(t)
	var got permission.Actor
	handler := NewAuth(auth).Wrap(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = permission.ActorFromContext(r.Context())
	}))
	session, err := auth.IssueSession(
		context.Background(), serviceauth.User{Email: "member@example.com", Sub: "member"},
		serviceauth.SignInMethodGoogle, "", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	request = request.WithContext(permission.ContextWithSystemActor(request.Context()))
	request.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: session})

	handler.ServeHTTP(httptest.NewRecorder(), request)

	if got.IsSystem() || got.Email != "member@example.com" {
		t.Fatalf("actor = %+v, want the authenticated member, not a pre-seeded system actor", got)
	}
}
