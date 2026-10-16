package httphandlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filesessions"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filetwofactor"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
)

const passwordHTTPCurrent = "current password secret"
const passwordHTTPNew = "replacement password secret"
const passwordHTTPEmail = "admin@example.com"

// These wrappers inject durable-store failures through the real service ports.
// They preserve every other operation so transport assertions exercise the
// complete workflow rather than stubbing the application result.
type passwordHTTPAuthStore struct {
	serviceauth.Store
	replaceErr error
}

func (s *passwordHTTPAuthStore) ReplaceLocalAdmin(ctx context.Context, expected, next serviceauth.LocalAdminCredential) error {
	if s.replaceErr != nil {
		return s.replaceErr
	}
	return s.Store.ReplaceLocalAdmin(ctx, expected, next)
}

type passwordHTTPRegistry struct {
	serviceauth.SessionRegistryStore
	saves, failSave int
}

func (s *passwordHTTPRegistry) Save(ctx context.Context, email string, record serviceauth.SessionRegistryRecord) error {
	s.saves++
	if s.saves == s.failSave {
		return errors.New(passwordHTTPCurrent + passwordHTTPNew)
	}
	return s.SessionRegistryStore.Save(ctx, email, record)
}

type passwordHTTPFactors struct {
	serviceauth.TwoFactorStore
	getErr, saveErr error
}

func (s *passwordHTTPFactors) Get(ctx context.Context, email string) (*serviceauth.TwoFactorRecord, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.TwoFactorStore.Get(ctx, email)
}
func (s *passwordHTTPFactors) Save(ctx context.Context, email string, record serviceauth.TwoFactorRecord) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.TwoFactorStore.Save(ctx, email, record)
}

type passwordHTTPFixture struct {
	jar      *cookieJar
	auth     *serviceauth.Service
	root     string
	store    *passwordHTTPAuthStore
	registry *passwordHTTPRegistry
	factors  *passwordHTTPFactors
}

func newPasswordHTTPFixture(t *testing.T) *passwordHTTPFixture {
	t.Helper()
	root := t.TempDir()
	factor, err := filetwofactor.New(root)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := filesessions.New(root)
	if err != nil {
		t.Fatal(err)
	}
	f := &passwordHTTPFixture{root: root, store: &passwordHTTPAuthStore{Store: fileauth.New(root)}, registry: &passwordHTTPRegistry{SessionRegistryStore: registry}, factors: &passwordHTTPFactors{TwoFactorStore: factor}}
	f.auth = f.newService(t)
	mux := http.NewServeMux()
	NewAuthHandler(f.auth, nil).RegisterRoutes(mux)
	NewSecurityHandler(f.auth).RegisterRoutes(mux)
	mux.Handle("/api/projects", httpmiddleware.NewAuth(f.auth).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })))
	f.jar = newCookieJar(mux)
	rec := f.jar.do(t, http.MethodPost, "/auth/local/claim", map[string]string{"email": passwordHTTPEmail, "password": passwordHTTPCurrent, "setupToken": setupTokenFor(t, f.auth)})
	if rec.Code != http.StatusCreated {
		t.Fatalf("claim: %d %s", rec.Code, rec.Body.String())
	}
	return f
}
func (f *passwordHTTPFixture) newService(t *testing.T) *serviceauth.Service {
	t.Helper()
	auth, err := serviceauth.New(context.Background(), f.store, twoFactorTestDirectory{}, func(string, string, string) serviceauth.OAuthProvider { return twoFactorTestOAuth{} }, "https://remote.example.com", []byte("test-session-key"), f.factors, f.registry, accountCredentialStoreForTest(t), serviceauth.Options{PendingLoginTTL: 5 * time.Minute, EnrollmentTTL: 10 * time.Minute, RecoveryCodeCount: 10, SessionHistoryLimit: 20, SetupTokenTTL: 30 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return auth
}
func (f *passwordHTTPFixture) enableTwoFactor(t *testing.T) string {
	t.Helper()
	rec := f.jar.do(t, http.MethodPost, "/api/me/security/2fa/enroll", nil)
	enrollment := decodeJSON[struct {
		EnrollmentToken string `json:"enrollmentToken"`
		Secret          string `json:"secret"`
	}](t, rec)
	// Enrollment's secret is standard unpadded base32.
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	rec = f.jar.do(t, http.MethodPost, "/api/me/security/2fa/confirm", map[string]string{"enrollmentToken": enrollment.EnrollmentToken, "code": serviceauth.TOTPCode(secret, time.Now())})
	if rec.Code != 200 {
		t.Fatalf("confirm: %d %s", rec.Code, rec.Body.String())
	}
	return decodeJSON[struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}](t, rec).RecoveryCodes[0]
}
func passwordHTTPBody() map[string]string {
	return map[string]string{"currentPassword": passwordHTTPCurrent, "newPassword": passwordHTTPNew}
}

func TestPasswordChangeHTTPErrorContract(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setup   func(*testing.T, *passwordHTTPFixture, map[string]string)
		status  int
		message string
	}{
		{name: "wrong current", setup: func(_ *testing.T, _ *passwordHTTPFixture, b map[string]string) { b["currentPassword"] = "incorrect" }, status: 403, message: serviceauth.ErrCurrentPasswordIncorrect.Error()},
		{name: "reuse", setup: func(_ *testing.T, _ *passwordHTTPFixture, b map[string]string) {
			b["newPassword"] = passwordHTTPCurrent
		}, status: 400, message: serviceauth.ErrPasswordReuse.Error()},
		{name: "short", setup: func(_ *testing.T, _ *passwordHTTPFixture, b map[string]string) { b["newPassword"] = "short" }, status: 400, message: serviceauth.ErrPasswordTooShort.Error()},
		{name: "long", setup: func(_ *testing.T, _ *passwordHTTPFixture, b map[string]string) {
			b["newPassword"] = strings.Repeat("x", 1025)
		}, status: 400, message: serviceauth.ErrPasswordTooLong.Error()},
		{name: "missing 2FA", setup: func(t *testing.T, f *passwordHTTPFixture, _ map[string]string) { f.enableTwoFactor(t) }, status: 400, message: serviceauth.ErrTwoFactorCodeRequired.Error()},
		{name: "wrong 2FA", setup: func(t *testing.T, f *passwordHTTPFixture, b map[string]string) {
			f.enableTwoFactor(t)
			b["code"] = "invalid"
		}, status: 403, message: serviceauth.ErrInvalidTwoFactorCode.Error()},
		{name: "2FA read fails", setup: func(_ *testing.T, f *passwordHTTPFixture, _ map[string]string) {
			f.factors.getErr = errors.New(passwordHTTPCurrent + passwordHTTPNew)
		}, status: 500, message: serviceauth.ErrPasswordNotChanged.Error()},
		{name: "2FA save fails", setup: func(t *testing.T, f *passwordHTTPFixture, b map[string]string) {
			b["code"] = f.enableTwoFactor(t)
			f.factors.saveErr = errors.New(passwordHTTPCurrent + passwordHTTPNew)
		}, status: 500, message: serviceauth.ErrPasswordNotChanged.Error()},
		{name: "revocation fails", setup: func(_ *testing.T, f *passwordHTTPFixture, _ map[string]string) {
			f.registry.failSave = f.registry.saves + 1
		}, status: 500, message: serviceauth.ErrPasswordNotChanged.Error()},
		{name: "credential save fails", setup: func(_ *testing.T, f *passwordHTTPFixture, _ map[string]string) {
			f.store.replaceErr = errors.New(passwordHTTPCurrent + passwordHTTPNew)
		}, status: 500, message: serviceauth.ErrPasswordNotChangedSessionsRevoked.Error()},
		{name: "concurrent credential", setup: func(_ *testing.T, f *passwordHTTPFixture, _ map[string]string) {
			f.store.replaceErr = serviceauth.ErrLocalAdminCredentialChanged
		}, status: 409, message: "password was changed elsewhere; reload and try again"},
		{name: "session renewal fails", setup: func(t *testing.T, f *passwordHTTPFixture, _ map[string]string) {
			rec := f.jar.do(t, http.MethodPost, "/api/me/security/preferences", map[string]bool{"historyEnabled": true})
			if rec.Code != 200 {
				t.Fatal(rec.Body.String())
			}
			f.registry.failSave = f.registry.saves + 2
		}, status: 500, message: serviceauth.ErrPasswordChangedSessionFailed.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPasswordHTTPFixture(t)
			body := passwordHTTPBody()
			tc.setup(t, f, body)
			rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", body)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.status, rec.Body.String())
			}
			if got := decodeJSON[map[string]string](t, rec)["error"]; got != tc.message {
				t.Fatalf("error=%q want=%q", got, tc.message)
			}
			if strings.Contains(rec.Body.String(), passwordHTTPCurrent) || strings.Contains(rec.Body.String(), passwordHTTPNew) {
				t.Fatal("password leaked in error")
			}
			if len(rec.Result().Cookies()) != 0 {
				t.Fatal("failed change mutated cookies")
			}
			if tc.status == 403 {
				if rec := f.jar.do(t, http.MethodGet, "/api/me/security", nil); rec.Code != 200 {
					t.Fatalf("proof error signed user out: %d", rec.Code)
				}
			}
		})
	}
}

// Mint a genuine legacy-format cookie with a historical iat without sleeping
// across a wall-clock boundary. Production CurrentSession verifies the MAC.
func passwordHTTPOldCookie(t *testing.T, email string) *http.Cookie {
	t.Helper()
	body, err := json.Marshal(serviceauth.Session{Email: email, Sub: "local-admin", Iat: time.Now().Add(-time.Minute).Unix(), Exp: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte("test-session-key"))
	mac.Write([]byte(payload))
	return &http.Cookie{Name: serviceauth.SessionCookieName, Value: payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))}
}
func TestPasswordChangeHTTPSuccessAndDurability(t *testing.T) {
	f := newPasswordHTTPFixture(t)
	old := passwordHTTPOldCookie(t, passwordHTTPEmail)
	f.jar.cookies[old.Name] = old
	if rec := f.jar.do(t, http.MethodGet, "/api/projects", nil); rec.Code != 200 {
		t.Fatalf("historical cookie invalid before change: %d %s", rec.Code, rec.Body.String())
	}
	summary := decodeJSON[serviceauth.SecuritySummary](t, f.jar.do(t, http.MethodGet, "/api/me/security", nil))
	if !summary.PasswordSignIn {
		t.Fatal("local account has no password sign-in")
	}
	rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", passwordHTTPBody())
	if rec.Code != 200 {
		t.Fatalf("change: %d %s", rec.Code, rec.Body.String())
	}
	if status := decodeJSON[serviceauth.Status](t, rec); !status.Authenticated || status.Email != passwordHTTPEmail {
		t.Fatalf("status=%+v", status)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != serviceauth.SessionCookieName || cookie.Value == old.Value || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != f.auth.CookieDomain() || cookie.MaxAge != int(serviceauth.SessionDuration().Seconds()) {
		t.Fatalf("unexpected cookie: %+v", cookie)
	}
	if rec := f.jar.do(t, http.MethodGet, "/api/projects", nil); rec.Code != 200 {
		t.Fatalf("fresh cookie rejected: %d", rec.Code)
	}
	stale := newCookieJar(f.jar.mux)
	stale.cookies[old.Name] = old
	if rec := stale.do(t, http.MethodGet, "/api/projects", nil); rec.Code != 401 {
		t.Fatalf("old cookie accepted after change: %d", rec.Code)
	}
	restarted := f.newService(t)
	if _, err := restarted.CurrentSession(context.Background(), old.Value); err == nil {
		t.Fatal("old session accepted after restart")
	}
	if _, err := restarted.CurrentSession(context.Background(), cookie.Value); err != nil {
		t.Fatalf("new session rejected after restart: %v", err)
	}
	if _, err := restarted.CompletePasswordLogin(context.Background(), passwordHTTPEmail, passwordHTTPCurrent, "", ""); err == nil {
		t.Fatal("old password accepted after restart")
	}
	if result, err := restarted.CompletePasswordLogin(context.Background(), passwordHTTPEmail, passwordHTTPNew, "", ""); err != nil || !result.Completed {
		t.Fatalf("new password failed after restart: %+v %v", result, err)
	}
	if err := filepath.WalkDir(f.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), passwordHTTPCurrent) || strings.Contains(string(data), passwordHTTPNew) {
			t.Errorf("plaintext password in %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestPasswordChangeHTTPGoogleOnly(t *testing.T) {
	f := newPasswordHTTPFixture(t)
	cookie, err := f.auth.IssueSession(context.Background(), serviceauth.User{Email: "google@example.com", Sub: "google-id"}, serviceauth.SignInMethodGoogle, "", "")
	if err != nil {
		t.Fatal(err)
	}
	f.jar.cookies[serviceauth.SessionCookieName] = &http.Cookie{Name: serviceauth.SessionCookieName, Value: cookie}
	summary := decodeJSON[serviceauth.SecuritySummary](t, f.jar.do(t, http.MethodGet, "/api/me/security", nil))
	if summary.PasswordSignIn {
		t.Fatal("Google-only account reports password sign-in")
	}
	rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", passwordHTTPBody())
	if rec.Code != 409 || decodeJSON[map[string]string](t, rec)["error"] != serviceauth.ErrPasswordSignInUnavailable.Error() {
		t.Fatalf("Google change: %d %s", rec.Code, rec.Body.String())
	}
}
func TestPasswordChangeHTTPRateLimit(t *testing.T) {
	f := newPasswordHTTPFixture(t)
	body := passwordHTTPBody()
	body["currentPassword"] = "incorrect"
	for i := 0; i < 5; i++ {
		rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", body)
		if rec.Code != 403 {
			t.Fatalf("failure %d: %d %s", i+1, rec.Code, rec.Body.String())
		}
	}
	rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", passwordHTTPBody())
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "300" || decodeJSON[map[string]string](t, rec)["error"] != "too many attempts; try again in a few minutes" {
		t.Fatalf("rate limit: %d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
	if rec := f.jar.do(t, http.MethodGet, "/api/me/security", nil); rec.Code != 200 {
		t.Fatal("rate limit signed user out")
	}
}
func TestPasswordChangeHTTPBoundary(t *testing.T) {
	f := newPasswordHTTPFixture(t)
	unauth := newCookieJar(f.jar.mux)
	rec := unauth.do(t, http.MethodPost, "/api/me/security/password", passwordHTTPBody())
	if rec.Code != 401 || decodeJSON[map[string]string](t, rec)["error"] != "authentication required" {
		t.Fatalf("unauth: %d %s", rec.Code, rec.Body.String())
	}
	rec = f.jar.do(t, http.MethodGet, "/api/me/security/password", nil)
	if rec.Code != 405 {
		t.Fatalf("method: %d", rec.Code)
	}
	for _, body := range []string{"{", "[]", "null", `{"currentPassword":123}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/me/security/password", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(f.jar.cookies[serviceauth.SessionCookieName])
		rec := httptest.NewRecorder()
		f.jar.mux.ServeHTTP(rec, req)
		if rec.Code != 400 {
			t.Fatalf("body %q: %d %s", body, rec.Code, rec.Body.String())
		}
	}
}

func TestPasswordChangeHTTPRecoveryCodeConsumed(t *testing.T) {
	f := newPasswordHTTPFixture(t)
	code := f.enableTwoFactor(t)
	body := passwordHTTPBody()
	body["code"] = code
	rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", body)
	if rec.Code != 200 {
		t.Fatalf("recovery change: %d %s", rec.Code, rec.Body.String())
	}
	summary := decodeJSON[serviceauth.SecuritySummary](t, f.jar.do(t, http.MethodGet, "/api/me/security", nil))
	if !summary.TwoFactorEnabled || summary.RecoveryCodesRemaining != 9 {
		t.Fatalf("factor state changed unexpectedly: %+v", summary)
	}
	body["currentPassword"] = passwordHTTPNew
	body["newPassword"] = "third replacement password secret"
	rec = f.jar.do(t, http.MethodPost, "/api/me/security/password", body)
	if rec.Code != 403 || decodeJSON[map[string]string](t, rec)["error"] != serviceauth.ErrInvalidTwoFactorCode.Error() {
		t.Fatalf("reused recovery code: %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.jar.do(t, http.MethodGet, "/api/me/security", nil); rec.Code != 200 {
		t.Fatal("recovery proof failure signed user out")
	}
}
func TestPasswordChangeHTTPInvalidTwoFactorCountsTowardRateLimit(t *testing.T) {
	f := newPasswordHTTPFixture(t)
	validCode := f.enableTwoFactor(t)
	body := passwordHTTPBody()
	body["code"] = "invalid"
	for i := 0; i < 5; i++ {
		if rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", body); rec.Code != 403 {
			t.Fatalf("failure %d: %d %s", i+1, rec.Code, rec.Body.String())
		}
	}
	body["code"] = validCode
	if rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", body); rec.Code != 429 {
		t.Fatalf("2FA attempts escaped limiter: %d %s", rec.Code, rec.Body.String())
	}
	// A different client IP has its own bucket even for the same account.
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/me/security/password", strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.17:4242"
	req.AddCookie(f.jar.cookies[serviceauth.SessionCookieName])
	rec := httptest.NewRecorder()
	f.jar.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("separate client denied: %d %s", rec.Code, rec.Body.String())
	}
}
func TestPasswordChangeHTTPValidationDoesNotCountAndSuccessResetsLimiter(t *testing.T) {
	f := newPasswordHTTPFixture(t)
	body := passwordHTTPBody()
	body["currentPassword"] = "incorrect"
	for i := 0; i < 4; i++ {
		if rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", body); rec.Code != 403 {
			t.Fatalf("initial proof failure: %d", rec.Code)
		}
	}
	invalid := passwordHTTPBody()
	invalid["newPassword"] = "short"
	for i := 0; i < 6; i++ {
		if rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", invalid); rec.Code != 400 {
			t.Fatalf("validation consumed limiter budget: %d %s", rec.Code, rec.Body.String())
		}
	}
	if rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", passwordHTTPBody()); rec.Code != 200 {
		t.Fatalf("valid change denied: %d %s", rec.Code, rec.Body.String())
	}
	for i := 0; i < 5; i++ {
		if rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", body); rec.Code != 403 {
			t.Fatalf("success did not reset limiter, failure %d: %d %s", i+1, rec.Code, rec.Body.String())
		}
	}
	if rec := f.jar.do(t, http.MethodPost, "/api/me/security/password", body); rec.Code != 429 {
		t.Fatalf("expected exhausted reset bucket, got %d", rec.Code)
	}
	// Password-change failures must not exhaust the separate login limiter.
	login := newCookieJar(f.jar.mux)
	if rec := login.do(t, http.MethodPost, "/auth/local/login", map[string]string{"email": passwordHTTPEmail, "password": passwordHTTPNew}); rec.Code != 200 {
		t.Fatalf("change limiter affected login: %d %s", rec.Code, rec.Body.String())
	}
}
