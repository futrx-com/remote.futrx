package httphandlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

// claimAndEnroll drives a fresh mux through claim + full 2FA enrollment and
// returns the jar holding the authenticated session, the TOTP secret, and the
// one-time recovery codes.
func claimAndEnroll(t *testing.T, mux *http.ServeMux, email, password string) (*cookieJar, []byte, []string) {
	t.Helper()
	jar := newCookieJar(mux)
	if rec := jar.do(t, http.MethodPost, "/auth/local/claim", map[string]string{
		"email": email, "password": password,
	}); rec.Code != http.StatusCreated {
		t.Fatalf("claim status = %d, body = %s", rec.Code, rec.Body.String())
	}
	enroll := decodeJSON[struct {
		EnrollmentToken string `json:"enrollmentToken"`
		Secret          string `json:"secret"`
	}](t, jar.do(t, http.MethodPost, "/api/me/security/2fa/enroll", nil))
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enroll.Secret)
	if err != nil {
		t.Fatalf("decode secret: %v", err)
	}
	confirmed := decodeJSON[struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}](t, jar.do(t, http.MethodPost, "/api/me/security/2fa/confirm", map[string]string{
		"enrollmentToken": enroll.EnrollmentToken,
		"code":            serviceauth.TOTPCode(secret, time.Now()),
	}))
	if len(confirmed.RecoveryCodes) == 0 {
		t.Fatal("confirm returned no recovery codes")
	}
	return jar, secret, confirmed.RecoveryCodes
}

// TestPendingTokenIsNotAcceptedAsSessionCookie is the end-to-end regression
// guard for the 2FA bypass: a caller who knows only the password receives a
// pending token, and replaying it as remote_session must not authenticate.
func TestPendingTokenIsNotAcceptedAsSessionCookie(t *testing.T) {
	mux := newTwoFactorTestMux(t)
	const email = "admin@example.com"
	const password = "correct horse battery staple"
	claimAndEnroll(t, mux, email, password)

	attacker := newCookieJar(mux)
	loginRec := attacker.do(t, http.MethodPost, "/auth/local/login", map[string]string{
		"email": email, "password": password,
	})
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d", loginRec.Code)
	}
	pending, ok := attacker.cookies[serviceauth.PendingTwoFactorCookieName]
	if !ok {
		t.Fatalf("expected a pending 2FA cookie, got %v", attacker.cookies)
	}
	if _, gotSession := attacker.cookies[serviceauth.SessionCookieName]; gotSession {
		t.Fatal("a real session cookie was issued before the second factor")
	}

	attacker.cookies = map[string]*http.Cookie{
		serviceauth.SessionCookieName: {Name: serviceauth.SessionCookieName, Value: pending.Value},
	}
	if rec := attacker.do(t, http.MethodGet, "/api/me/security", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("pending token replayed as a session: status = %d, body = %s (2FA bypassed)", rec.Code, rec.Body.String())
	}
}

// TestSecurityEndpointsRejectAnonymousCallers keeps every /api/me/security
// route behind a session.
func TestSecurityEndpointsRejectAnonymousCallers(t *testing.T) {
	mux := newTwoFactorTestMux(t)
	const email = "admin@example.com"
	claimAndEnroll(t, mux, email, "correct horse battery staple")

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/me/security"},
		{http.MethodPost, "/api/me/security/2fa/enroll"},
		{http.MethodPost, "/api/me/security/2fa/confirm"},
		{http.MethodPost, "/api/me/security/2fa/disable"},
		{http.MethodPost, "/api/me/security/2fa/recovery-codes/regenerate"},
		{http.MethodPost, "/api/me/security/preferences"},
		{http.MethodPost, "/api/me/security/alerts/ack"},
	} {
		t.Run(route.path, func(t *testing.T) {
			anon := newCookieJar(mux)
			if rec := anon.do(t, route.method, route.path, map[string]string{}); rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s anonymous status = %d, want 401", route.method, route.path, rec.Code)
			}
		})
	}
}

// TestSecurityEndpointsRejectForgedSessionCookie proves the routes rely on a
// verified signature, not merely on a cookie being present.
func TestSecurityEndpointsRejectForgedSessionCookie(t *testing.T) {
	mux := newTwoFactorTestMux(t)
	claimAndEnroll(t, mux, "admin@example.com", "correct horse battery staple")

	forged := newCookieJar(mux)
	forged.cookies = map[string]*http.Cookie{
		serviceauth.SessionCookieName: {Name: serviceauth.SessionCookieName, Value: "eyJlbWFpbCI6ImFkbWluQGV4YW1wbGUuY29tIn0.bogus"},
	}
	if rec := forged.do(t, http.MethodGet, "/api/me/security", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged cookie status = %d, want 401", rec.Code)
	}
}

// TestTwoFactorVerifyWithoutPendingChallengeIsRejected stops the challenge
// endpoint from being driven without a first factor.
func TestTwoFactorVerifyWithoutPendingChallengeIsRejected(t *testing.T) {
	mux := newTwoFactorTestMux(t)
	_, secret, _ := claimAndEnroll(t, mux, "admin@example.com", "correct horse battery staple")

	anon := newCookieJar(mux)
	rec := anon.do(t, http.MethodPost, "/auth/2fa/verify", map[string]string{
		"code": serviceauth.TOTPCode(secret, time.Now()),
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("verify without a pending challenge status = %d, want 401", rec.Code)
	}
	if _, got := anon.cookies[serviceauth.SessionCookieName]; got {
		t.Fatal("a session cookie was issued without a pending challenge")
	}
}

// TestTwoFactorCancelClearsPendingChallenge covers the documented cancel flow.
func TestTwoFactorCancelClearsPendingChallenge(t *testing.T) {
	mux := newTwoFactorTestMux(t)
	const email = "admin@example.com"
	const password = "correct horse battery staple"
	_, secret, _ := claimAndEnroll(t, mux, email, password)

	client := newCookieJar(mux)
	client.do(t, http.MethodPost, "/auth/local/login", map[string]string{"email": email, "password": password})
	if _, ok := client.cookies[serviceauth.PendingTwoFactorCookieName]; !ok {
		t.Fatal("no pending cookie after first factor")
	}
	if rec := client.do(t, http.MethodPost, "/auth/2fa/cancel", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("cancel status = %d, want 204", rec.Code)
	}
	if _, ok := client.cookies[serviceauth.PendingTwoFactorCookieName]; ok {
		t.Fatal("pending cookie survived cancel")
	}
	if rec := client.do(t, http.MethodPost, "/auth/2fa/verify", map[string]string{
		"code": serviceauth.TOTPCode(secret, time.Now()),
	}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("verify after cancel status = %d, want 401", rec.Code)
	}
}

// TestTwoFactorVerifyIsRateLimited pins the brute-force guard on a 6-digit
// secret-independent code space.
func TestTwoFactorVerifyIsRateLimited(t *testing.T) {
	mux := newTwoFactorTestMux(t)
	const email = "admin@example.com"
	const password = "correct horse battery staple"
	claimAndEnroll(t, mux, email, password)

	client := newCookieJar(mux)
	client.do(t, http.MethodPost, "/auth/local/login", map[string]string{"email": email, "password": password})

	sawRateLimit := false
	for i := range 12 {
		rec := client.do(t, http.MethodPost, "/auth/2fa/verify", map[string]string{"code": "000000"})
		if rec.Code == http.StatusTooManyRequests {
			sawRateLimit = true
			break
		}
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401 or 429", i, rec.Code)
		}
	}
	if !sawRateLimit {
		t.Fatal("2FA verify never rate-limited across 12 wrong codes")
	}
}

// TestPasswordAndTwoFactorFailuresShareRateLimit keeps both stages of local
// authentication inside the documented five-failure per-IP bucket.
func TestPasswordAndTwoFactorFailuresShareRateLimit(t *testing.T) {
	mux := newTwoFactorTestMux(t)
	const email = "admin@example.com"
	const password = "correct horse battery staple"
	claimAndEnroll(t, mux, email, password)

	client := newCookieJar(mux)
	if rec := client.do(t, http.MethodPost, "/auth/local/login", map[string]string{
		"email": email, "password": password,
	}); rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}

	for attempt := range 4 {
		rec := client.do(t, http.MethodPost, "/auth/local/login", map[string]string{
			"email": email, "password": "not-the-password",
		})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("password attempt %d status = %d, want 401", attempt+1, rec.Code)
		}
	}

	if rec := client.do(t, http.MethodPost, "/auth/2fa/verify", map[string]string{
		"code": "not-a-valid-code",
	}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("fifth shared attempt status = %d, want 401", rec.Code)
	}
	if rec := client.do(t, http.MethodPost, "/auth/2fa/verify", map[string]string{
		"code": "not-a-valid-code",
	}); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt after five shared failures status = %d, want 429", rec.Code)
	}
}

// TestConfirmEnrollmentRejectsAnotherAccountsToken makes sure a token minted
// for one account cannot enroll a secret onto the calling account (or the
// token's account) - and that nothing is persisted when it is rejected.
func TestConfirmEnrollmentRejectsAnotherAccountsToken(t *testing.T) {
	mux := newTwoFactorTestMux(t)
	jar := newCookieJar(mux)
	const email = "admin@example.com"
	if rec := jar.do(t, http.MethodPost, "/auth/local/claim", map[string]string{
		"email": email, "password": "correct horse battery staple",
	}); rec.Code != http.StatusCreated {
		t.Fatalf("claim status = %d", rec.Code)
	}

	// A token minted for a different account, signed with the same key.
	foreign := foreignEnrollmentToken(t, "victim@example.com")
	rec := jar.do(t, http.MethodPost, "/api/me/security/2fa/confirm", map[string]string{
		"enrollmentToken": foreign.token,
		"code":            serviceauth.TOTPCode(foreign.secret, time.Now()),
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-account confirm status = %d, want 403", rec.Code)
	}
	summary := decodeJSON[serviceauth.SecuritySummary](t, jar.do(t, http.MethodGet, "/api/me/security", nil))
	if summary.TwoFactorEnabled {
		t.Fatal("a rejected cross-account confirm still enrolled 2FA")
	}
}

// TestDisableThenLoginNeedsNoSecondFactor is the full opt-out round trip.
func TestDisableThenLoginNeedsNoSecondFactor(t *testing.T) {
	mux := newTwoFactorTestMux(t)
	const email = "admin@example.com"
	const password = "correct horse battery staple"
	jar, secret, _ := claimAndEnroll(t, mux, email, password)

	if rec := jar.do(t, http.MethodPost, "/api/me/security/2fa/disable", map[string]string{
		"code": serviceauth.TOTPCode(secret, time.Now()),
	}); rec.Code != http.StatusNoContent {
		t.Fatalf("disable status = %d, body = %s", rec.Code, rec.Body.String())
	}

	client := newCookieJar(mux)
	rec := client.do(t, http.MethodPost, "/auth/local/login", map[string]string{"email": email, "password": password})
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d", rec.Code)
	}
	if _, ok := client.cookies[serviceauth.SessionCookieName]; !ok {
		t.Fatal("login after disabling 2FA did not issue a session cookie")
	}
	if _, ok := client.cookies[serviceauth.PendingTwoFactorCookieName]; ok {
		t.Fatal("login after disabling 2FA still demanded a second factor")
	}
}

// TestWrongPasswordNeverStartsATwoFactorChallenge keeps the first factor
// authoritative.
func TestWrongPasswordNeverStartsATwoFactorChallenge(t *testing.T) {
	mux := newTwoFactorTestMux(t)
	const email = "admin@example.com"
	claimAndEnroll(t, mux, email, "correct horse battery staple")

	client := newCookieJar(mux)
	rec := client.do(t, http.MethodPost, "/auth/local/login", map[string]string{
		"email": email, "password": "not-the-password",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad-password login status = %d, want 401", rec.Code)
	}
	if _, ok := client.cookies[serviceauth.PendingTwoFactorCookieName]; ok {
		t.Fatal("a wrong password produced a pending 2FA challenge")
	}
}

// foreignEnrollmentToken mints a valid enrollment token for someone other
// than the calling account, signed with the same key the test mux uses. It
// rebuilds the envelope by hand (base64(json) "." base64(HMAC)) because the
// codec is private to the auth package; that also pins the wire format the
// cross-account guard depends on.
type mintedEnrollment struct {
	token  string
	secret []byte
}

func foreignEnrollmentToken(t *testing.T, email string) mintedEnrollment {
	t.Helper()
	secret, err := serviceauth.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	body, err := json.Marshal(struct {
		Email  string `json:"email"`
		Secret []byte `json:"secret"`
		Exp    int64  `json:"exp"`
	}{Email: email, Secret: secret, Exp: time.Now().Add(10 * time.Minute).Unix()})
	if err != nil {
		t.Fatalf("marshal enrollment: %v", err)
	}
	b64 := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte("test-session-key"))
	mac.Write([]byte("2fa-enrollment/v1"))
	mac.Write([]byte{0})
	mac.Write([]byte(b64))
	return mintedEnrollment{
		token:  b64 + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)),
		secret: secret,
	}
}
