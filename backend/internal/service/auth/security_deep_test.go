package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// enrollTestAccount drives an account through the full 2FA enrollment so the
// deep cases below start from a confirmed, real enrollment rather than a
// hand-built record.
func enrollServiceAccount(t *testing.T, service *Service, email string) (secret []byte, recoveryCodes []string) {
	t.Helper()
	ctx := context.Background()
	token, _, _, err := service.BeginTwoFactorEnrollment(ctx, email)
	if err != nil {
		t.Fatalf("BeginTwoFactorEnrollment: %v", err)
	}
	pending, err := service.twoFactor.codec.verify(token)
	if err != nil {
		t.Fatalf("verify enrollment token: %v", err)
	}
	codes, _, err := service.ConfirmTwoFactorEnrollment(ctx, email, token, TOTPCode(pending.Secret, time.Now()))
	if err != nil {
		t.Fatalf("ConfirmTwoFactorEnrollment: %v", err)
	}
	return pending.Secret, codes
}

// --- Token domain separation ------------------------------------------------

// TestPendingLoginTokenIsNotAValidSession is the regression guard for the 2FA
// bypass: pendingLogin and Session are signed with the same key and their
// JSON overlaps, so without domain separation a pending token decodes into a
// fully valid session and the second factor is skipped.
func TestPendingLoginTokenIsNotAValidSession(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	enrollServiceAccount(t, service, email)

	result, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("CompletePasswordLogin: %v", err)
	}
	if result.Completed || result.PendingToken == "" {
		t.Fatalf("expected a pending 2FA result, got %+v", result)
	}
	if _, err := service.CurrentSession(ctx, result.PendingToken); err == nil {
		t.Fatal("pending 2FA token was accepted as a session cookie: 2FA can be bypassed")
	}
}

// TestSessionCookieIsNotAValidPendingLogin closes the mirror direction, so a
// real long-lived session cannot be replayed into the challenge endpoint.
func TestSessionCookieIsNotAValidPendingLogin(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	secret, _ := enrollServiceAccount(t, service, email)

	cookie, err := service.IssueSession(ctx, User{Email: email, Sub: "local-admin"}, SignInMethodPassword, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}
	if _, err := service.CompleteTwoFactorChallenge(ctx, cookie, TOTPCode(secret, time.Now()), "1.2.3.4", "ua"); !errors.Is(err, ErrInvalidPendingLogin) {
		t.Fatalf("session cookie accepted as a pending login token, err = %v", err)
	}
}

// TestEnrollmentTokenIsNotAValidSession covers the third payload type sharing
// the session key.
func TestEnrollmentTokenIsNotAValidSession(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	token, _, _, err := service.BeginTwoFactorEnrollment(ctx, email)
	if err != nil {
		t.Fatalf("BeginTwoFactorEnrollment: %v", err)
	}
	if _, err := service.CurrentSession(ctx, token); err == nil {
		t.Fatal("enrollment token was accepted as a session cookie")
	}
}

// TestExistingSessionsSurviveDomainSeparation pins the compatibility promise:
// the session codec uses the empty domain, so a cookie signed the pre-change
// way still verifies and nobody is logged out by the fix.
func TestExistingSessionsSurviveDomainSeparation(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	legacy := newSessionPayload([]byte("test-session-key"))
	now := time.Now()
	raw := legacy.sign(Session{Email: email, Sub: "local-admin", Iat: now.Unix(), Exp: now.Add(time.Hour).Unix()})
	if _, err := service.CurrentSession(context.Background(), raw); err != nil {
		t.Fatalf("pre-domain session cookie no longer verifies: %v", err)
	}
}

// --- Signature and payload tampering ---------------------------------------

func TestTamperedSessionCookiesAreRejected(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	cookie, err := service.IssueSession(ctx, User{Email: email, Sub: "local-admin"}, SignInMethodPassword, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}
	parts := strings.SplitN(cookie, ".", 2)

	cases := map[string]string{
		"empty":            "",
		"no separator":     parts[0],
		"body only":        parts[0] + ".",
		"signature only":   "." + parts[1],
		"swapped halves":   parts[1] + "." + parts[0],
		"flipped sig char": parts[0] + "." + flipLast(parts[1]),
		"flipped body":     flipLast(parts[0]) + "." + parts[1],
		"garbage":          "not-a-token",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := service.CurrentSession(ctx, value); err == nil {
				t.Fatalf("tampered cookie %q was accepted", value)
			}
		})
	}
}

func flipLast(s string) string {
	if s == "" {
		return "x"
	}
	last := s[len(s)-1]
	repl := byte('A')
	if last == 'A' {
		repl = 'B'
	}
	return s[:len(s)-1] + string(repl)
}

// TestForgedSessionWithDifferentKeyIsRejected proves the cookie is not merely
// decoded but authenticated.
func TestForgedSessionWithDifferentKeyIsRejected(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	forged := newSessionPayload([]byte("attacker-key"))
	now := time.Now()
	raw := forged.sign(Session{Email: email, Sub: "local-admin", Iat: now.Unix(), Exp: now.Add(time.Hour).Unix()})
	if _, err := service.CurrentSession(context.Background(), raw); err == nil {
		t.Fatal("session signed with a foreign key was accepted")
	}
}

// --- Expiry -----------------------------------------------------------------

func TestExpiredSessionAndPendingTokensAreRejected(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()

	expiredSession := newSessionPayload([]byte("test-session-key")).
		sign(Session{Email: email, Sub: "local-admin", Iat: time.Now().Add(-2 * time.Hour).Unix(), Exp: time.Now().Add(-time.Hour).Unix()})
	if _, err := service.CurrentSession(ctx, expiredSession); err == nil {
		t.Fatal("expired session cookie was accepted")
	}

	secret, _ := enrollServiceAccount(t, service, email)
	expiredPending := service.pendingLoginCodec.sign(pendingLogin{
		Email: email, Sub: "local-admin", Method: SignInMethodPassword,
		Exp: time.Now().Add(-time.Minute).Unix(),
	})
	if _, err := service.CompleteTwoFactorChallenge(ctx, expiredPending, TOTPCode(secret, time.Now()), "1.2.3.4", "ua"); !errors.Is(err, ErrInvalidPendingLogin) {
		t.Fatalf("expired pending login accepted, err = %v", err)
	}
}

func TestExpiredEnrollmentTokenIsRejected(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	stale := service.twoFactor.codec.sign(pendingEnrollment{
		Email: email, Secret: secret, Exp: time.Now().Add(-time.Second).Unix(),
	})
	if _, _, err := service.ConfirmTwoFactorEnrollment(context.Background(), email, stale, TOTPCode(secret, time.Now())); !errors.Is(err, ErrInvalidEnrollmentToken) {
		t.Fatalf("expired enrollment token accepted, err = %v", err)
	}
}

// --- Second-factor correctness ---------------------------------------------

// TestWrongTOTPCodeNeverCompletesLogin walks a spread of wrong shapes so a
// permissive parser cannot slip through.
func TestWrongTOTPCodeNeverCompletesLogin(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	secret, _ := enrollServiceAccount(t, service, email)

	result, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("CompletePasswordLogin: %v", err)
	}

	wrong := []string{
		"", "0", "00000", "0000000", "abcdef", "  ", "------",
		"null", "true", "%00", "000000",
	}
	correct := TOTPCode(secret, time.Now())
	for _, code := range wrong {
		if code == correct {
			continue
		}
		if _, err := service.CompleteTwoFactorChallenge(ctx, result.PendingToken, code, "1.2.3.4", "ua"); err == nil {
			t.Fatalf("challenge completed with wrong code %q", code)
		}
	}
	if _, err := service.CompleteTwoFactorChallenge(ctx, result.PendingToken, correct, "1.2.3.4", "ua"); err != nil {
		t.Fatalf("correct code rejected after failed attempts: %v", err)
	}
}

// TestTOTPCodeFromAnotherAccountIsRejected guards cross-account confusion.
func TestTOTPCodeFromAnotherAccountIsRejected(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	enrollServiceAccount(t, service, email)

	otherSecret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	result, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("CompletePasswordLogin: %v", err)
	}
	if _, err := service.CompleteTwoFactorChallenge(ctx, result.PendingToken, TOTPCode(otherSecret, time.Now()), "1.2.3.4", "ua"); err == nil {
		t.Fatal("a TOTP code from a different secret completed the challenge")
	}
}

// TestTOTPSkewWindowBoundaries pins the accepted window to exactly +/-1 step.
func TestTOTPSkewWindowBoundaries(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	// Anchor mid-step so +/-30s cannot straddle two boundaries at once.
	base := time.Unix((time.Now().Unix()/30)*30+15, 0)
	for _, tc := range []struct {
		name   string
		offset time.Duration
		want   bool
	}{
		{"current step", 0, true},
		{"one step back", -30 * time.Second, true},
		{"one step forward", 30 * time.Second, true},
		{"two steps back", -60 * time.Second, false},
		{"two steps forward", 60 * time.Second, false},
		{"far future", 10 * time.Minute, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := TOTPCode(secret, base.Add(tc.offset))
			if got := verifyTOTPCode(secret, code, base); got != tc.want {
				t.Fatalf("verifyTOTPCode(offset %v) = %v, want %v", tc.offset, got, tc.want)
			}
		})
	}
}

// --- Recovery codes ---------------------------------------------------------

// TestRecoveryCodeIsSingleUse is the core anti-replay promise of a recovery code.
func TestRecoveryCodeIsSingleUse(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	_, codes := enrollServiceAccount(t, service, email)

	first, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("CompletePasswordLogin: %v", err)
	}
	if _, err := service.CompleteTwoFactorChallenge(ctx, first.PendingToken, codes[0], "1.2.3.4", "ua"); err != nil {
		t.Fatalf("first recovery-code use failed: %v", err)
	}
	second, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("CompletePasswordLogin (2nd): %v", err)
	}
	if _, err := service.CompleteTwoFactorChallenge(ctx, second.PendingToken, codes[0], "1.2.3.4", "ua"); err == nil {
		t.Fatal("a recovery code was accepted twice")
	}
	if got := service.twoFactor.RecoveryCodesRemaining(ctx, email); got != len(codes)-1 {
		t.Fatalf("RecoveryCodesRemaining = %d, want %d", got, len(codes)-1)
	}
}

// TestRecoveryCodeNormalizationIsAccepted mirrors how people actually retype
// a code, without loosening what counts as a match.
func TestRecoveryCodeNormalizationIsAccepted(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	_, codes := enrollServiceAccount(t, service, email)

	result, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("CompletePasswordLogin: %v", err)
	}
	messy := "  " + strings.ToLower(codes[0]) + "  "
	if _, err := service.CompleteTwoFactorChallenge(ctx, result.PendingToken, messy, "1.2.3.4", "ua"); err != nil {
		t.Fatalf("lower-cased, padded recovery code rejected: %v", err)
	}
}

// TestRecoveryCodesAreDistinctAndWellFormed sanity-checks generation.
func TestRecoveryCodesAreDistinctAndWellFormed(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	_, codes := enrollServiceAccount(t, service, email)
	if len(codes) != service.twoFactor.recoveryCodeCount {
		t.Fatalf("got %d recovery codes, want %d", len(codes), service.twoFactor.recoveryCodeCount)
	}
	seen := map[string]struct{}{}
	for _, c := range codes {
		if _, dup := seen[c]; dup {
			t.Fatalf("duplicate recovery code %q", c)
		}
		seen[c] = struct{}{}
		if len(strings.Split(c, "-")) != recoveryCodeGroups {
			t.Fatalf("recovery code %q is not %d groups", c, recoveryCodeGroups)
		}
		for _, ch := range strings.ReplaceAll(c, "-", "") {
			if !strings.ContainsRune(recoveryCodeAlphabet, ch) {
				t.Fatalf("recovery code %q contains out-of-alphabet %q", c, ch)
			}
		}
	}
}

// TestRegenerateRecoveryCodesInvalidatesOldOnes ensures rotation really rotates.
func TestRegenerateRecoveryCodesInvalidatesOldOnes(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	secret, old := enrollServiceAccount(t, service, email)

	fresh, err := service.RegenerateRecoveryCodes(ctx, email, TOTPCode(secret, time.Now()))
	if err != nil {
		t.Fatalf("RegenerateRecoveryCodes: %v", err)
	}
	result, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("CompletePasswordLogin: %v", err)
	}
	if _, err := service.CompleteTwoFactorChallenge(ctx, result.PendingToken, old[0], "1.2.3.4", "ua"); err == nil {
		t.Fatal("an old recovery code still worked after regeneration")
	}
	if _, err := service.CompleteTwoFactorChallenge(ctx, result.PendingToken, fresh[0], "1.2.3.4", "ua"); err != nil {
		t.Fatalf("a regenerated recovery code was rejected: %v", err)
	}
}

// TestRegenerateRecoveryCodesRequiresValidTOTP stops a hijacked session from
// silently minting itself a fresh set of codes.
func TestRegenerateRecoveryCodesRequiresValidTOTP(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	enrollServiceAccount(t, service, email)
	if _, err := service.RegenerateRecoveryCodes(ctx, email, "000000"); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("RegenerateRecoveryCodes with a bad code err = %v, want ErrInvalidTwoFactorCode", err)
	}
}

// --- Disable ----------------------------------------------------------------

func TestDisableTwoFactorRequiresProofOfPossession(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	secret, _ := enrollServiceAccount(t, service, email)

	if err := service.DisableTwoFactor(ctx, email, "000000"); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("disable with a wrong code err = %v, want ErrInvalidTwoFactorCode", err)
	}
	if !service.TwoFactorEnabled(ctx, email) {
		t.Fatal("2FA was disabled despite a rejected code")
	}
	if err := service.DisableTwoFactor(ctx, email, TOTPCode(secret, time.Now())); err != nil {
		t.Fatalf("DisableTwoFactor: %v", err)
	}
	if service.TwoFactorEnabled(ctx, email) {
		t.Fatal("2FA still enabled after a successful disable")
	}
	// After disabling, a password login completes in one step again.
	result, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("CompletePasswordLogin: %v", err)
	}
	if !result.Completed {
		t.Fatal("login still demanded a second factor after 2FA was disabled")
	}
}

// TestDisabledTwoFactorInvalidatesOutstandingPendingLogin makes sure a token
// minted before the account changed cannot be redeemed afterwards.
func TestDisabledTwoFactorInvalidatesOutstandingPendingLogin(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	secret, codes := enrollServiceAccount(t, service, email)

	result, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("CompletePasswordLogin: %v", err)
	}
	if err := service.DisableTwoFactor(ctx, email, TOTPCode(secret, time.Now())); err != nil {
		t.Fatalf("DisableTwoFactor: %v", err)
	}
	if _, err := service.CompleteTwoFactorChallenge(ctx, result.PendingToken, codes[0], "1.2.3.4", "ua"); !errors.Is(err, ErrTwoFactorNotEnabled) {
		t.Fatalf("stale pending login err = %v, want ErrTwoFactorNotEnabled", err)
	}
}

// TestEnrollmentCannotBeStartedTwice guards the already-enabled path.
func TestEnrollmentCannotBeStartedTwice(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	enrollServiceAccount(t, service, email)
	if _, _, _, err := service.BeginTwoFactorEnrollment(ctx, email); !errors.Is(err, ErrTwoFactorAlreadyEnabled) {
		t.Fatalf("second BeginTwoFactorEnrollment err = %v, want ErrTwoFactorAlreadyEnabled", err)
	}
}

// --- Single active session --------------------------------------------------

func TestSingleSessionSupersedesPreviousSession(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	if err := service.SetSecurityPreferences(ctx, email, SecurityPreferences{SingleSessionEnabled: true}); err != nil {
		t.Fatalf("SetSecurityPreferences: %v", err)
	}

	first, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.1.1.1", "ua-1")
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	if _, err := service.CurrentSession(ctx, first.CookieValue); err != nil {
		t.Fatalf("first session should be active: %v", err)
	}
	second, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "2.2.2.2", "ua-2")
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if _, err := service.CurrentSession(ctx, first.CookieValue); !errors.Is(err, ErrSessionSuperseded) {
		t.Fatalf("first session err = %v, want ErrSessionSuperseded", err)
	}
	if _, err := service.CurrentSession(ctx, second.CookieValue); err != nil {
		t.Fatalf("second session should be active: %v", err)
	}
}

// TestSingleSessionAppliesWithoutTwoFactor pins the documented promise that
// supersession is independent of 2FA.
func TestSingleSessionAppliesWithoutTwoFactor(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	if service.TwoFactorEnabled(ctx, email) {
		t.Fatal("precondition: 2FA should be off")
	}
	if err := service.SetSecurityPreferences(ctx, email, SecurityPreferences{SingleSessionEnabled: true}); err != nil {
		t.Fatalf("SetSecurityPreferences: %v", err)
	}
	first, _ := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.1.1.1", "ua-1")
	if _, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "2.2.2.2", "ua-2"); err != nil {
		t.Fatalf("second login: %v", err)
	}
	if _, err := service.CurrentSession(ctx, first.CookieValue); !errors.Is(err, ErrSessionSuperseded) {
		t.Fatalf("first session err = %v, want ErrSessionSuperseded with 2FA off", err)
	}
}

// TestRevokeSessionEndsTheActiveSession covers logout.
func TestRevokeSessionEndsTheActiveSession(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	if err := service.SetSecurityPreferences(ctx, email, SecurityPreferences{SingleSessionEnabled: true}); err != nil {
		t.Fatalf("SetSecurityPreferences: %v", err)
	}
	login, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.1.1.1", "ua")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	legacyCookie := service.codec.sign(User{Email: email, Sub: "local-admin"}, "")
	if err := service.RevokeSession(ctx, email); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if _, err := service.CurrentSession(ctx, login.CookieValue); !errors.Is(err, ErrSessionSuperseded) {
		t.Fatalf("revoked session err = %v, want ErrSessionSuperseded", err)
	}
	if _, err := service.CurrentSession(ctx, legacyCookie); !errors.Is(err, ErrSessionSuperseded) {
		t.Fatalf("legacy empty-SID session err = %v, want ErrSessionSuperseded", err)
	}
}

// TestTurningSingleSessionOffStopsEnforcing verifies the toggle is reversible.
func TestTurningSingleSessionOffStopsEnforcing(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	if err := service.SetSecurityPreferences(ctx, email, SecurityPreferences{SingleSessionEnabled: true}); err != nil {
		t.Fatalf("SetSecurityPreferences: %v", err)
	}
	first, _ := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.1.1.1", "ua-1")
	if _, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "2.2.2.2", "ua-2"); err != nil {
		t.Fatalf("second login: %v", err)
	}
	if err := service.SetSecurityPreferences(ctx, email, SecurityPreferences{}); err != nil {
		t.Fatalf("SetSecurityPreferences (off): %v", err)
	}
	if _, err := service.CurrentSession(ctx, first.CookieValue); err != nil {
		t.Fatalf("with single-session off the older cookie should verify again: %v", err)
	}
}

// --- Sign-in history --------------------------------------------------------

func TestHistoryIsNewestFirstAndBounded(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	if err := service.SetSecurityPreferences(ctx, email, SecurityPreferences{HistoryEnabled: true}); err != nil {
		t.Fatalf("SetSecurityPreferences: %v", err)
	}
	historyLimit := service.registry.historyLimit
	total := historyLimit + 5
	for i := range total {
		if _, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, ipForIndex(i), uaForIndex(i)); err != nil {
			t.Fatalf("login %d: %v", i, err)
		}
	}
	summary, err := service.SecuritySummary(ctx, email)
	if err != nil {
		t.Fatalf("SecuritySummary: %v", err)
	}
	if len(summary.Sessions) != historyLimit {
		t.Fatalf("history length = %d, want cap %d", len(summary.Sessions), historyLimit)
	}
	if got, want := summary.Sessions[0].UserAgent, uaForIndex(total-1); got != want {
		t.Fatalf("newest entry UA = %q, want %q (newest-first)", got, want)
	}
	for i := 1; i < len(summary.Sessions); i++ {
		if summary.Sessions[i-1].IssuedAt < summary.Sessions[i].IssuedAt {
			t.Fatalf("history not newest-first at index %d", i)
		}
	}
	// Session ids must be unique across issuances.
	seen := map[string]struct{}{}
	for _, s := range summary.Sessions {
		if s.SID == "" {
			t.Fatal("history entry has an empty session id")
		}
		if _, dup := seen[s.SID]; dup {
			t.Fatalf("duplicate session id %q in history", s.SID)
		}
		seen[s.SID] = struct{}{}
	}
}

func ipForIndex(i int) string { return "10.0.0." + itoa(i%250) }
func uaForIndex(i int) string { return "ua-" + itoa(i) }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// TestHistoryDisabledRecordsNothing keeps the opt-in promise.
func TestHistoryDisabledRecordsNothing(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	if _, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.1.1.1", "ua"); err != nil {
		t.Fatalf("login: %v", err)
	}
	summary, err := service.SecuritySummary(ctx, email)
	if err != nil {
		t.Fatalf("SecuritySummary: %v", err)
	}
	if len(summary.Sessions) != 0 {
		t.Fatalf("history recorded %d entries while disabled", len(summary.Sessions))
	}
}

// TestHistoryRecordsTheCombinedSignInMethod checks the method labelling that
// the recovery-code alert keys off.
func TestHistoryRecordsTheCombinedSignInMethod(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	secret, codes := enrollServiceAccount(t, service, email)
	if err := service.SetSecurityPreferences(ctx, email, SecurityPreferences{HistoryEnabled: true}); err != nil {
		t.Fatalf("SetSecurityPreferences: %v", err)
	}

	totpLogin, _ := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.1.1.1", "ua-totp")
	if _, err := service.CompleteTwoFactorChallenge(ctx, totpLogin.PendingToken, TOTPCode(secret, time.Now()), "1.1.1.1", "ua-totp"); err != nil {
		t.Fatalf("totp challenge: %v", err)
	}
	recoveryLogin, _ := service.CompletePasswordLogin(ctx, email, testAdminPassword, "2.2.2.2", "ua-recovery")
	if _, err := service.CompleteTwoFactorChallenge(ctx, recoveryLogin.PendingToken, codes[0], "2.2.2.2", "ua-recovery"); err != nil {
		t.Fatalf("recovery challenge: %v", err)
	}

	summary, err := service.SecuritySummary(ctx, email)
	if err != nil {
		t.Fatalf("SecuritySummary: %v", err)
	}
	if len(summary.Sessions) < 2 {
		t.Fatalf("expected at least 2 history entries, got %d", len(summary.Sessions))
	}
	if summary.Sessions[0].Method != SignInMethodPasswordRecoveryCode {
		t.Fatalf("newest method = %q, want %q", summary.Sessions[0].Method, SignInMethodPasswordRecoveryCode)
	}
	if summary.Sessions[1].Method != SignInMethodPasswordTOTP {
		t.Fatalf("previous method = %q, want %q", summary.Sessions[1].Method, SignInMethodPasswordTOTP)
	}
}

// --- Recovery-code alert ----------------------------------------------------

func TestRecoveryCodeAlertFiresOnlyForRecoveryLogins(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	secret, codes := enrollServiceAccount(t, service, email)
	if err := service.SetSecurityPreferences(ctx, email, SecurityPreferences{RecoveryCodeAlertEnabled: true}); err != nil {
		t.Fatalf("SetSecurityPreferences: %v", err)
	}

	totpLogin, _ := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.1.1.1", "ua")
	if _, err := service.CompleteTwoFactorChallenge(ctx, totpLogin.PendingToken, TOTPCode(secret, time.Now()), "1.1.1.1", "ua"); err != nil {
		t.Fatalf("totp challenge: %v", err)
	}
	summary, _ := service.SecuritySummary(ctx, email)
	if summary.SecurityAlert != nil {
		t.Fatal("a TOTP login raised a recovery-code alert")
	}

	recoveryLogin, _ := service.CompletePasswordLogin(ctx, email, testAdminPassword, "9.9.9.9", "ua-evil")
	if _, err := service.CompleteTwoFactorChallenge(ctx, recoveryLogin.PendingToken, codes[0], "9.9.9.9", "ua-evil"); err != nil {
		t.Fatalf("recovery challenge: %v", err)
	}
	summary, _ = service.SecuritySummary(ctx, email)
	if summary.SecurityAlert == nil {
		t.Fatal("recovery-code login did not raise an alert")
	}
	if summary.SecurityAlert.IP != "9.9.9.9" || summary.SecurityAlert.UserAgent != "ua-evil" {
		t.Fatalf("alert lost its device context: %+v", summary.SecurityAlert)
	}
	if summary.SecurityAlert.Method != SignInMethodPasswordRecoveryCode {
		t.Fatalf("alert method = %q", summary.SecurityAlert.Method)
	}

	if err := service.AckSecurityAlert(ctx, email); err != nil {
		t.Fatalf("AckSecurityAlert: %v", err)
	}
	summary, _ = service.SecuritySummary(ctx, email)
	if summary.SecurityAlert != nil {
		t.Fatal("alert survived acknowledgement")
	}
}

// TestAlertDisabledSuppressesAlert keeps the third toggle independent.
func TestAlertDisabledSuppressesAlert(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	_, codes := enrollServiceAccount(t, service, email)
	if err := service.SetSecurityPreferences(ctx, email, SecurityPreferences{HistoryEnabled: true}); err != nil {
		t.Fatalf("SetSecurityPreferences: %v", err)
	}
	login, _ := service.CompletePasswordLogin(ctx, email, testAdminPassword, "9.9.9.9", "ua")
	if _, err := service.CompleteTwoFactorChallenge(ctx, login.PendingToken, codes[0], "9.9.9.9", "ua"); err != nil {
		t.Fatalf("recovery challenge: %v", err)
	}
	summary, _ := service.SecuritySummary(ctx, email)
	if summary.SecurityAlert != nil {
		t.Fatal("alert raised while the alert toggle was off")
	}
}

// --- Concurrency ------------------------------------------------------------

// TestConcurrentRecoveryCodeUseConsumesItOnce is the double-spend guard: the
// load/verify/save sequence must not let two racing requests redeem the same
// single-use code.
func TestConcurrentRecoveryCodeUseConsumesItOnce(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	_, codes := enrollServiceAccount(t, service, email)

	const racers = 8
	var wg sync.WaitGroup
	successes := make(chan bool, racers)
	start := make(chan struct{})
	for range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			used, err := service.twoFactor.VerifyChallenge(ctx, email, codes[0])
			successes <- err == nil && used
		}()
	}
	close(start)
	wg.Wait()
	close(successes)

	accepted := 0
	for ok := range successes {
		if ok {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("a single recovery code was redeemed %d times concurrently, want exactly 1", accepted)
	}
	if got := service.twoFactor.RecoveryCodesRemaining(ctx, email); got != len(codes)-1 {
		t.Fatalf("RecoveryCodesRemaining = %d, want %d", got, len(codes)-1)
	}
}

// TestConcurrentLoginsKeepHistoryConsistent guards the same read-modify-write
// window on the registry side.
func TestConcurrentLoginsKeepHistoryConsistent(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	if err := service.SetSecurityPreferences(ctx, email, SecurityPreferences{HistoryEnabled: true}); err != nil {
		t.Fatalf("SetSecurityPreferences: %v", err)
	}
	const logins = 10
	var wg sync.WaitGroup
	for i := range logins {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.CompletePasswordLogin(ctx, email, testAdminPassword, ipForIndex(i), uaForIndex(i)); err != nil {
				t.Errorf("login %d: %v", i, err)
			}
		}()
	}
	wg.Wait()

	summary, err := service.SecuritySummary(ctx, email)
	if err != nil {
		t.Fatalf("SecuritySummary: %v", err)
	}
	if len(summary.Sessions) != logins {
		t.Fatalf("history recorded %d of %d concurrent logins", len(summary.Sessions), logins)
	}
}

// TestConfirmEnrollmentDoesNotWriteAnotherAccountsRecord is the regression
// guard for enrolling a secret onto someone else's account. The mismatch must
// be caught before anything is persisted, not after: a later rejection still
// leaves the victim enrolled with an attacker-chosen secret.
func TestConfirmEnrollmentDoesNotWriteAnotherAccountsRecord(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	const victim = "victim@example.com"

	victimToken, _, _, err := service.BeginTwoFactorEnrollment(ctx, victim)
	if err != nil {
		t.Fatalf("BeginTwoFactorEnrollment: %v", err)
	}
	pending, err := service.twoFactor.codec.verify(victimToken)
	if err != nil {
		t.Fatalf("verify enrollment token: %v", err)
	}

	// The local admin, authenticated as itself, presents the victim's token.
	_, _, err = service.ConfirmTwoFactorEnrollment(ctx, email, victimToken, TOTPCode(pending.Secret, time.Now()))
	if !errors.Is(err, ErrEnrollmentTokenMismatch) {
		t.Fatalf("cross-account confirm err = %v, want ErrEnrollmentTokenMismatch", err)
	}
	if service.TwoFactorEnabled(ctx, victim) {
		t.Fatal("a rejected cross-account confirm still enrolled 2FA on the victim's account")
	}
	if service.TwoFactorEnabled(ctx, email) {
		t.Fatal("a rejected cross-account confirm enrolled 2FA on the caller's account")
	}
}

// --- TOTP replay ------------------------------------------------------------

// TestTOTPCodeCannotBeReplayed is the RFC 6238 SS5.2 guard: a code observed
// over someone's shoulder stays valid for the rest of its ~90s window unless
// first use consumes it.
func TestTOTPCodeCannotBeReplayed(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	secret, _ := enrollServiceAccount(t, service, email)
	code := TOTPCode(secret, time.Now())

	first, _ := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.1.1.1", "ua")
	if _, err := service.CompleteTwoFactorChallenge(ctx, first.PendingToken, code, "1.1.1.1", "ua"); err != nil {
		t.Fatalf("first use of a fresh code failed: %v", err)
	}
	second, _ := service.CompletePasswordLogin(ctx, email, testAdminPassword, "9.9.9.9", "ua-evil")
	if _, err := service.CompleteTwoFactorChallenge(ctx, second.PendingToken, code, "9.9.9.9", "ua-evil"); !errors.Is(err, ErrTwoFactorCodeReused) {
		t.Fatalf("replayed code err = %v, want ErrTwoFactorCodeReused", err)
	}
}

// TestNextTOTPCodeStillWorksAfterReplayGuard makes sure replay prevention
// does not lock an honest user out on their next sign-in.
func TestNextTOTPCodeStillWorksAfterReplayGuard(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	secret, _ := enrollServiceAccount(t, service, email)

	now := time.Now()
	first, _ := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.1.1.1", "ua")
	if _, err := service.CompleteTwoFactorChallenge(ctx, first.PendingToken, TOTPCode(secret, now), "1.1.1.1", "ua"); err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	// Advancing by exactly one step always lands on counter+1, which is both
	// a different counter and still inside the +/-1 skew window.
	nextCode := TOTPCode(secret, now.Add(totpStep))
	if nextCode == TOTPCode(secret, now) {
		t.Skip("adjacent steps produced the same 6-digit code; nothing to distinguish")
	}
	used, err := service.twoFactor.VerifyChallenge(ctx, email, nextCode)
	if err != nil {
		t.Fatalf("next-step code rejected: %v", err)
	}
	if used {
		t.Fatal("a TOTP code was reported as a recovery code")
	}
}

// TestReplayGuardDoesNotAffectRecoveryCodes keeps the two second-factor paths
// independent.
func TestReplayGuardDoesNotAffectRecoveryCodes(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	secret, codes := enrollServiceAccount(t, service, email)

	first, _ := service.CompletePasswordLogin(ctx, email, testAdminPassword, "1.1.1.1", "ua")
	if _, err := service.CompleteTwoFactorChallenge(ctx, first.PendingToken, TOTPCode(secret, time.Now()), "1.1.1.1", "ua"); err != nil {
		t.Fatalf("totp sign-in: %v", err)
	}
	second, _ := service.CompletePasswordLogin(ctx, email, testAdminPassword, "2.2.2.2", "ua")
	if _, err := service.CompleteTwoFactorChallenge(ctx, second.PendingToken, codes[0], "2.2.2.2", "ua"); err != nil {
		t.Fatalf("a distinct recovery code was rejected after a TOTP sign-in: %v", err)
	}
}

// TestLegacyRecordWithoutReplayCounterStillAccepts pins backward compatibility
// for records persisted before replay tracking existed.
func TestLegacyRecordWithoutReplayCounterStillAccepts(t *testing.T) {
	service, email := newTestServiceWithLocalAdmin(t)
	ctx := context.Background()
	secret, _ := enrollServiceAccount(t, service, email)

	// Rewrite the record the way an older build would have stored it.
	legacy := TwoFactorRecord{Secret: secret, RecoveryCodeHashes: []string{}, EnabledAt: time.Now().Unix()}
	if err := service.twoFactor.store.Save(ctx, email, legacy); err != nil {
		t.Fatalf("save legacy record: %v", err)
	}
	service.twoFactor.setCache(email, &legacy)

	if _, err := service.twoFactor.VerifyChallenge(ctx, email, TOTPCode(secret, time.Now())); err != nil {
		t.Fatalf("legacy record (no replay counter) rejected a valid code: %v", err)
	}
}
