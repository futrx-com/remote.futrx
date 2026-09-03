package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filesessions"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filetwofactor"
)

// newLocalAuthTestMux builds a minimal http.ServeMux with the auth handlers
// wired up and an in-memory / temp-dir store. The returned mux is unclaimed
// (no local admin credential exists yet).
func newLocalAuthTestMux(t *testing.T) *http.ServeMux {
	t.Helper()
	twoFactorStore, err := filetwofactor.New(t.TempDir())
	if err != nil {
		t.Fatalf("init two-factor store: %v", err)
	}
	sessionRegistryStore, err := filesessions.New(t.TempDir())
	if err != nil {
		t.Fatalf("init session registry store: %v", err)
	}
	auth, err := serviceauth.New(
		context.Background(),
		fileauth.New(t.TempDir()),
		twoFactorTestDirectory{}, // reuses the stub from auth_twofactor_handler_test.go
		func(string, string, string) serviceauth.OAuthProvider { return twoFactorTestOAuth{} },
		"https://remote.example.com",
		[]byte("test-session-key"),
		twoFactorStore,
		sessionRegistryStore,
		serviceauth.Options{
			PendingLoginTTL:     5 * time.Minute,
			EnrollmentTTL:       10 * time.Minute,
			RecoveryCodeCount:   10,
			SessionHistoryLimit: 20,
		},
	)
	if err != nil {
		t.Fatalf("New auth service: %v", err)
	}
	mux := http.NewServeMux()
	NewAuthHandler(auth, nil).RegisterRoutes(mux)
	return mux
}

// postClaim fires POST /auth/local/claim from the given remoteAddr (IP:port).
func postClaim(t *testing.T, mux *http.ServeMux, remoteAddr, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/auth/local/claim", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// TestClaimRejectsRemoteCallers is the regression guard required by issue #65:
// the /auth/local/claim endpoint must not be accessible from non-loopback
// addresses to prevent any network-adjacent user from racing the operator to
// set the admin credential.
func TestClaimRejectsRemoteCallers(t *testing.T) {
	remoteAddrs := []string{
		"192.168.1.42:54321",    // LAN
		"10.0.0.1:12345",       // private range
		"203.0.113.1:80",       // public internet
		"2001:db8::1:80",       // IPv6 public
	}
	for _, addr := range remoteAddrs {
		t.Run(addr, func(t *testing.T) {
			mux := newLocalAuthTestMux(t)
			rec := postClaim(t, mux, addr, "admin@example.com", "correct horse battery staple")
			if rec.Code != http.StatusForbidden {
				t.Errorf("claim from remote addr %q: got %d, want %d (forbidden)", addr, rec.Code, http.StatusForbidden)
			}
		})
	}
}

// TestClaimAllowsLocalhostCallers verifies that a request from the loopback
// interface is still accepted (the setup can be performed from the terminal).
func TestClaimAllowsLocalhostCallers(t *testing.T) {
	loopbackAddrs := []struct {
		name string
		addr string
	}{
		{"IPv4 loopback", "127.0.0.1:54321"},
		{"IPv6 loopback", "[::1]:54321"},
		{"localhost range", "127.0.0.2:12345"},
	}
	for _, tc := range loopbackAddrs {
		t.Run(tc.name, func(t *testing.T) {
			mux := newLocalAuthTestMux(t)
			rec := postClaim(t, mux, tc.addr, "admin@example.com", "correct horse battery staple")
			if rec.Code != http.StatusCreated {
				t.Errorf("claim from loopback %q: got %d (body: %s), want %d (created)",
					tc.addr, rec.Code, rec.Body.String(), http.StatusCreated)
			}
		})
	}
}

// TestMeClaimAllowedOnlyForLoopback verifies that /auth/me sets claimAllowed
// to true only when the server is unclaimed and the caller is on localhost.
func TestMeClaimAllowedOnlyForLoopback(t *testing.T) {
	cases := []struct {
		name        string
		remoteAddr  string
		wantAllowed bool
	}{
		{"loopback IPv4", "127.0.0.1:54321", true},
		{"loopback IPv6", "[::1]:54321", true},
		{"remote LAN", "192.168.1.42:54321", false},
		{"remote public", "203.0.113.5:80", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := newLocalAuthTestMux(t)
			req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
			req.RemoteAddr = tc.remoteAddr
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET /auth/me status = %d", rec.Code)
			}
			var status struct {
				Claimed      bool `json:"claimed"`
				ClaimAllowed bool `json:"claimAllowed"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
				t.Fatalf("decode /auth/me: %v", err)
			}
			if status.Claimed {
				t.Fatal("expected unclaimed server for this test")
			}
			if status.ClaimAllowed != tc.wantAllowed {
				t.Errorf("claimAllowed = %v, want %v (remoteAddr=%q)", status.ClaimAllowed, tc.wantAllowed, tc.remoteAddr)
			}
		})
	}
}
