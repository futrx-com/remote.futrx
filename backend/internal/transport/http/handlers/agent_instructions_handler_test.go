package httphandlers

import (
	"context"
	"encoding/json"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type instructionStoreSpy struct{ writes int }

func (s *instructionStoreSpy) Read() (json.RawMessage, error) { return json.RawMessage(`{}`), nil }
func (s *instructionStoreSpy) Write([]byte) error             { s.writes++; return nil }
func (s *instructionStoreSpy) Targets() map[string]string     { return nil }
func TestInstructionsRejectUnauthenticatedAndNonAdminWrites(t *testing.T) {
	_, auth, _ := newClaimTestServer(t)
	token, err := auth.IssueSession(context.Background(), serviceauth.User{Email: "member@example.com", Sub: "member"}, serviceauth.SignInMethodGoogle, "", "")
	if err != nil {
		t.Fatal(err)
	}
	store := &instructionStoreSpy{}
	mux := http.NewServeMux()
	NewAgentInstructionsHandler(store, auth).RegisterRoutes(mux)
	for _, tc := range []struct {
		cookie string
		code   int
	}{{"", 401}, {token, 403}} {
		req := httptest.NewRequest("PUT", "/api/admin/agent-instructions", strings.NewReader(`{"global":"override"}`))
		if tc.cookie != "" {
			req.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: tc.cookie})
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.code {
			t.Fatalf("status %d want %d", rec.Code, tc.code)
		}
	}
	if store.writes != 0 {
		t.Fatal("unauthorized settings written")
	}
}

type instructionAdminDirectory struct{ claimTestDirectory }

func (*instructionAdminDirectory) IsAdmin(context.Context, string) (bool, error) { return true, nil }
func TestInstructionsAdministratorReadWriteAndSizeLimit(t *testing.T) {
	auth, err := serviceauth.New(context.Background(), fileauth.New(t.TempDir()), &instructionAdminDirectory{}, func(string, string, string) serviceauth.OAuthProvider { return claimTestOAuth{} }, "https://remote.example.com", []byte("0123456789abcdef0123456789abcdef"), twoFactorStoreForTest(t), sessionRegistryStoreForTest(t), testAuthOptions())
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueSession(context.Background(), serviceauth.User{Email: "admin@example.com", Sub: "admin"}, serviceauth.SignInMethodGoogle, "", "")
	if err != nil {
		t.Fatal(err)
	}
	store := &instructionStoreSpy{}
	mux := http.NewServeMux()
	NewAgentInstructionsHandler(store, auth).RegisterRoutes(mux)
	for _, tc := range []struct {
		method, body string
		code         int
	}{{"GET", "", 200}, {"PUT", `{"global":"policy"}`, 200}, {"PUT", strings.Repeat("x", 1024*1024+1), 400}, {"DELETE", "", 405}} {
		req := httptest.NewRequest(tc.method, "/api/admin/agent-instructions", strings.NewReader(tc.body))
		req.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.code {
			t.Fatalf("%s got %d: %s", tc.method, rec.Code, rec.Body.String())
		}
	}
	if store.writes != 1 {
		t.Fatalf("writes = %d", store.writes)
	}
}
