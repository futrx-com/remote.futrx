package httphandlers

import (
	"context"
	"encoding/base32"
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

// twoFactorTestDirectory treats the claimed local admin as the only account
// that matters for these tests; Google login is never exercised here.
type twoFactorTestDirectory struct{}

func (twoFactorTestDirectory) IsAdmin(context.Context, string) (bool, error)      { return true, nil }
func (twoFactorTestDirectory) IsRegistered(context.Context, string) (bool, error) { return true, nil }
func (twoFactorTestDirectory) AddBootstrapAdmin(context.Context, string) error    { return nil }
func (twoFactorTestDirectory) FirstAdmin(context.Context) (*serviceauth.UserDirectoryEntry, error) {
	return nil, nil
}

type twoFactorTestOAuth struct{}

func (twoFactorTestOAuth) AuthCodeURL(string) string { return "https://accounts.example.com" }
func (twoFactorTestOAuth) ExchangeUser(context.Context, string) (serviceauth.User, error) {
	return serviceauth.User{}, nil
}

func newTwoFactorTestMux(t *testing.T) *http.ServeMux {
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
		twoFactorTestDirectory{},
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
	NewSecurityHandler(auth).RegisterRoutes(mux)
	return mux
}

// cookieJar is a minimal helper that remembers Set-Cookie headers across
// requests to the same test mux, so a test can drive a login/enroll/confirm
// sequence as a browser would.
type cookieJar struct {
	mux     *http.ServeMux
	cookies map[string]*http.Cookie
}

func newCookieJar(mux *http.ServeMux) *cookieJar {
	return &cookieJar{mux: mux, cookies: map[string]*http.Cookie{}}
}

func (j *cookieJar) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = strings.NewReader(string(data))
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range j.cookies {
		req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}
	rec := httptest.NewRecorder()
	j.mux.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(j.cookies, c.Name)
			continue
		}
		j.cookies[c.Name] = c
	}
	return rec
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestTwoFactorEnrollLoginAlertDisableRoundTrip(t *testing.T) {
	jar := newCookieJar(newTwoFactorTestMux(t))
	const email = "admin@example.com"
	const password = "correct horse battery staple"

	claimRec := jar.do(t, http.MethodPost, "/auth/local/claim", map[string]string{
		"email": email, "password": password,
	})
	if claimRec.Code != http.StatusCreated {
		t.Fatalf("claim status = %d, body = %s", claimRec.Code, claimRec.Body.String())
	}

	summary := decodeJSON[serviceauth.SecuritySummary](t, jar.do(t, http.MethodGet, "/api/me/security", nil))
	if summary.TwoFactorEnabled {
		t.Fatal("2FA reported enabled before enrollment")
	}

	// Turn on the recovery-code alert requires 2FA first - confirm the
	// independent-toggle guard rejects it while 2FA is off.
	prefRec := jar.do(t, http.MethodPost, "/api/me/security/preferences", map[string]bool{"recoveryCodeAlertEnabled": true})
	if prefRec.Code != http.StatusBadRequest {
		t.Fatalf("enabling the alert without 2FA status = %d, want %d", prefRec.Code, http.StatusBadRequest)
	}

	enrollRec := jar.do(t, http.MethodPost, "/api/me/security/2fa/enroll", nil)
	if enrollRec.Code != http.StatusOK {
		t.Fatalf("enroll status = %d, body = %s", enrollRec.Code, enrollRec.Body.String())
	}
	enroll := decodeJSON[struct {
		EnrollmentToken string `json:"enrollmentToken"`
		Secret          string `json:"secret"`
		OtpauthURL      string `json:"otpauthUrl"`
	}](t, enrollRec)
	if enroll.EnrollmentToken == "" || enroll.Secret == "" || enroll.OtpauthURL == "" {
		t.Fatalf("enroll response missing fields: %+v", enroll)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enroll.Secret)
	if err != nil {
		t.Fatalf("decode secret: %v", err)
	}

	confirmRec := jar.do(t, http.MethodPost, "/api/me/security/2fa/confirm", map[string]string{
		"enrollmentToken": enroll.EnrollmentToken,
		"code":            serviceauth.TOTPCode(secret, time.Now()),
	})
	if confirmRec.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, body = %s", confirmRec.Code, confirmRec.Body.String())
	}
	confirmed := decodeJSON[struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}](t, confirmRec)
	if len(confirmed.RecoveryCodes) == 0 {
		t.Fatal("confirm did not return recovery codes")
	}

	summary = decodeJSON[serviceauth.SecuritySummary](t, jar.do(t, http.MethodGet, "/api/me/security", nil))
	if !summary.TwoFactorEnabled {
		t.Fatal("2FA not enabled after confirm")
	}

	// Now that 2FA is on, the alert flag can be turned on.
	prefRec = jar.do(t, http.MethodPost, "/api/me/security/preferences", map[string]bool{"recoveryCodeAlertEnabled": true})
	if prefRec.Code != http.StatusOK {
		t.Fatalf("enabling the alert status = %d, body = %s", prefRec.Code, prefRec.Body.String())
	}

	// Logging in again requires the second factor.
	loginRec := jar.do(t, http.MethodPost, "/auth/local/login", map[string]string{"email": email, "password": password})
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRec.Code, loginRec.Body.String())
	}
	loginResp := decodeJSON[struct {
		TwoFactorRequired bool `json:"twoFactorRequired"`
	}](t, loginRec)
	if !loginResp.TwoFactorRequired {
		t.Fatal("login did not report twoFactorRequired for a 2FA-enabled account")
	}

	// Complete the challenge with a recovery code instead of a TOTP code.
	verifyRec := jar.do(t, http.MethodPost, "/auth/2fa/verify", map[string]string{"code": confirmed.RecoveryCodes[0]})
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("2fa verify status = %d, body = %s", verifyRec.Code, verifyRec.Body.String())
	}

	summary = decodeJSON[serviceauth.SecuritySummary](t, jar.do(t, http.MethodGet, "/api/me/security", nil))
	if summary.SecurityAlert == nil {
		t.Fatal("security summary did not surface the recovery-code alert")
	}
	if summary.SecurityAlert.Method != serviceauth.SignInMethodPasswordRecoveryCode {
		t.Fatalf("alert method = %q, want %q", summary.SecurityAlert.Method, serviceauth.SignInMethodPasswordRecoveryCode)
	}

	ackRec := jar.do(t, http.MethodPost, "/api/me/security/alerts/ack", nil)
	if ackRec.Code != http.StatusNoContent {
		t.Fatalf("ack status = %d", ackRec.Code)
	}
	summary = decodeJSON[serviceauth.SecuritySummary](t, jar.do(t, http.MethodGet, "/api/me/security", nil))
	if summary.SecurityAlert != nil {
		t.Fatalf("alert still present after ack: %+v", summary.SecurityAlert)
	}

	// Disable 2FA using a fresh TOTP code; the alert flag must be cleared
	// along with it since it cannot remain on without 2FA.
	disableRec := jar.do(t, http.MethodPost, "/api/me/security/2fa/disable", map[string]string{
		"code": serviceauth.TOTPCode(secret, time.Now()),
	})
	if disableRec.Code != http.StatusNoContent {
		t.Fatalf("disable status = %d, body = %s", disableRec.Code, disableRec.Body.String())
	}
	summary = decodeJSON[serviceauth.SecuritySummary](t, jar.do(t, http.MethodGet, "/api/me/security", nil))
	if summary.TwoFactorEnabled || summary.RecoveryCodeAlertEnabled {
		t.Fatalf("state after disable = %+v, want 2FA and alert both off", summary)
	}
}

func TestSingleSessionPreferenceSupersedesWithoutTwoFactor(t *testing.T) {
	jar := newCookieJar(newTwoFactorTestMux(t))
	const email = "admin@example.com"
	const password = "correct horse battery staple"

	if rec := jar.do(t, http.MethodPost, "/auth/local/claim", map[string]string{"email": email, "password": password}); rec.Code != http.StatusCreated {
		t.Fatalf("claim status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := jar.do(t, http.MethodPost, "/api/me/security/preferences", map[string]bool{"singleSessionEnabled": true}); rec.Code != http.StatusOK {
		t.Fatalf("enable single-session status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// A second "device" logging in against the same mux/service instance
	// supersedes the jar's tracked session cookie - drive the request
	// directly (not through the jar, so the jar's cookie stays what it was
	// right after claim/enable) and confirm the jar's stale cookie is then
	// rejected by a protected endpoint.
	loginBody, _ := json.Marshal(map[string]string{"email": email, "password": password})
	secondDeviceReq := httptest.NewRequest(http.MethodPost, "/auth/local/login", strings.NewReader(string(loginBody)))
	secondDeviceReq.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	jar.mux.ServeHTTP(secondRec, secondDeviceReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second device login status = %d, body = %s", secondRec.Code, secondRec.Body.String())
	}

	staleRec := jar.do(t, http.MethodGet, "/api/me/security", nil)
	if staleRec.Code != http.StatusUnauthorized {
		t.Fatalf("stale session status = %d, want %d (superseded)", staleRec.Code, http.StatusUnauthorized)
	}
}
