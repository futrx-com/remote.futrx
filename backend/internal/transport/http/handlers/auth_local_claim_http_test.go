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

// The claim gate is a security control reached over HTTP, so it needs a test
// that actually goes over HTTP. Everything below the handler is covered in the
// auth service; what is only reachable here is the request decoding and the
// error-to-status mapping - the seam where a renamed JSON field or a reordered
// switch case silently reopens the hole.

const claimHTTPPassword = "correct horse battery staple"

// claimTestDirectory is an empty user directory, which is what makes a claim
// token-gated: nobody exists to authorise it any other way.
type claimTestDirectory struct{ bootstrapped []string }

func (d *claimTestDirectory) IsAdmin(context.Context, string) (bool, error)      { return false, nil }
func (d *claimTestDirectory) IsRegistered(context.Context, string) (bool, error) { return false, nil }

func (d *claimTestDirectory) AddBootstrapAdmin(_ context.Context, email string) error {
	d.bootstrapped = append(d.bootstrapped, email)
	return nil
}

func (d *claimTestDirectory) FirstAdmin(context.Context) (*serviceauth.UserDirectoryEntry, error) {
	if len(d.bootstrapped) == 0 {
		return nil, nil
	}
	return &serviceauth.UserDirectoryEntry{Email: d.bootstrapped[0]}, nil
}

type claimTestOAuth struct{}

func (claimTestOAuth) AuthCodeURL(string) string { return "" }
func (claimTestOAuth) ExchangeUser(context.Context, string) (serviceauth.User, error) {
	return serviceauth.User{}, nil
}

// newClaimTestServer wires the real handler over a real auth service and a
// real on-disk store, so a claim that succeeds genuinely writes the credential.
func newClaimTestServer(t *testing.T) (*http.ServeMux, *serviceauth.Service, string) {
	t.Helper()
	dataDir := t.TempDir()
	store := fileauth.New(dataDir)
	twoFactorStore, err := filetwofactor.New(dataDir)
	if err != nil {
		t.Fatalf("init two-factor store: %v", err)
	}
	sessionRegistryStore, err := filesessions.New(dataDir)
	if err != nil {
		t.Fatalf("init session registry store: %v", err)
	}
	auth, err := serviceauth.New(
		context.Background(),
		store,
		&claimTestDirectory{},
		func(string, string, string) serviceauth.OAuthProvider { return claimTestOAuth{} },
		"https://remote.example.com",
		[]byte("0123456789abcdef0123456789abcdef"),
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
		t.Fatalf("auth.New: %v", err)
	}
	mux := http.NewServeMux()
	(&localAuthHandler{auth: auth, logins: newLocalLoginLimiter()}).RegisterRoutes(mux)
	return mux, auth, dataDir
}

func postClaim(t *testing.T, mux *http.ServeMux, clientIP, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/local/claim", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", clientIP)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	return res
}

func claimBody(t *testing.T, fields map[string]string) string {
	t.Helper()
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal claim body: %v", err)
	}
	return string(encoded)
}

func TestClaimOverHTTPRejectsAMissingSetupToken(t *testing.T) {
	mux, auth, _ := newClaimTestServer(t)
	if _, err := auth.EnsureSetupToken(context.Background()); err != nil {
		t.Fatalf("EnsureSetupToken: %v", err)
	}

	res := postClaim(t, mux, "10.0.0.1", claimBody(t, map[string]string{
		"email": "attacker@example.com", "password": claimHTTPPassword,
	}))

	if res.Code != http.StatusForbidden {
		t.Fatalf("tokenless claim status = %d, want 403; body = %s", res.Code, res.Body)
	}
	if auth.LocalAdminConfigured() {
		t.Fatal("a tokenless claim over HTTP configured the local admin")
	}
}

func TestClaimOverHTTPAcceptsTheIssuedTokenExactlyOnce(t *testing.T) {
	mux, auth, _ := newClaimTestServer(t)
	token, err := auth.EnsureSetupToken(context.Background())
	if err != nil || token == "" {
		t.Fatalf("EnsureSetupToken = %q, %v", token, err)
	}

	res := postClaim(t, mux, "10.0.0.2", claimBody(t, map[string]string{
		"email": "owner@example.com", "password": claimHTTPPassword, "setupToken": token,
	}))
	if res.Code != http.StatusCreated {
		t.Fatalf("valid claim status = %d, want 201; body = %s", res.Code, res.Body)
	}
	if !auth.LocalAdminConfigured() {
		t.Fatal("a successful claim did not configure the local admin")
	}
	// AC3: the token must not come back to the caller.
	if strings.Contains(res.Body.String(), token) {
		t.Fatalf("the setup token was echoed in the response: %s", res.Body)
	}

	replay := postClaim(t, mux, "10.0.0.3", claimBody(t, map[string]string{
		"email": "attacker@example.com", "password": "another secure password", "setupToken": token,
	}))
	if replay.Code != http.StatusConflict {
		t.Fatalf("replayed claim status = %d, want 409; body = %s", replay.Code, replay.Body)
	}
}

// The gate engages only if the handler reads the field the client actually
// sends. Renaming the struct tag would leave every service-level test green
// while every claim arrived tokenless.
func TestClaimOverHTTPReadsTheSetupTokenFieldByName(t *testing.T) {
	mux, auth, _ := newClaimTestServer(t)
	token, err := auth.EnsureSetupToken(context.Background())
	if err != nil {
		t.Fatalf("EnsureSetupToken: %v", err)
	}

	res := postClaim(t, mux, "10.0.0.4", claimBody(t, map[string]string{
		"email": "attacker@example.com", "password": claimHTTPPassword, "setup_token": token,
	}))

	if res.Code != http.StatusForbidden {
		t.Fatalf("claim carrying setup_token status = %d, want 403; body = %s", res.Code, res.Body)
	}
	if auth.LocalAdminConfigured() {
		t.Fatal("a claim naming the token field wrongly still configured the local admin")
	}
}

// A token in the query string would be logged by the reverse proxy, so the
// handler must ignore it there.
func TestClaimOverHTTPIgnoresASetupTokenInTheQueryString(t *testing.T) {
	mux, auth, _ := newClaimTestServer(t)
	token, err := auth.EnsureSetupToken(context.Background())
	if err != nil {
		t.Fatalf("EnsureSetupToken: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/auth/local/claim?setupToken="+token,
		strings.NewReader(claimBody(t, map[string]string{
			"email": "attacker@example.com", "password": claimHTTPPassword,
		})),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "10.0.0.5")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("query-string token status = %d, want 403; body = %s", res.Code, res.Body)
	}
	if auth.LocalAdminConfigured() {
		t.Fatal("a query-string token configured the local admin")
	}
}

// Repeated bad tokens must exhaust the limiter rather than allow unbounded
// guessing, and rotating the forwarded prefix must not buy a fresh bucket.
func TestClaimOverHTTPRateLimitsRepeatedBadTokens(t *testing.T) {
	mux, auth, _ := newClaimTestServer(t)
	if _, err := auth.EnsureSetupToken(context.Background()); err != nil {
		t.Fatalf("EnsureSetupToken: %v", err)
	}

	body := claimBody(t, map[string]string{
		"email": "attacker@example.com", "password": claimHTTPPassword, "setupToken": "wrong",
	})
	for i := range 5 {
		if code := postClaim(t, mux, "203.0.113.9", body).Code; code != http.StatusForbidden {
			t.Fatalf("attempt %d status = %d, want 403", i+1, code)
		}
	}
	if code := postClaim(t, mux, "203.0.113.9", body).Code; code != http.StatusTooManyRequests {
		t.Fatalf("sixth attempt status = %d, want 429", code)
	}
	// Same real client, a different spoofed prefix: still the same bucket.
	if code := postClaim(t, mux, "198.51.100.7, 203.0.113.9", body).Code; code != http.StatusTooManyRequests {
		t.Fatalf("rotated-prefix attempt status = %d, want 429", code)
	}
}
