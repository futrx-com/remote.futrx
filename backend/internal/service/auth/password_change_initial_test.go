package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	initialGoogleSub = "google-sub-1"
	initialNewSecret = "my brand new password"
)

// initialChangeToken signs in with the temporary password and returns the
// password-change token it earns.
func (f *invitedFixture) initialChangeToken(t *testing.T) string {
	t.Helper()
	result, err := f.service.CompletePasswordLogin(context.Background(), invitedEmail, invitedTemp, "1.2.3.4", "browser")
	if err != nil || !result.PasswordChangeRequired || result.PasswordChangeToken == "" {
		t.Fatalf("CompletePasswordLogin = %+v, %v", result, err)
	}
	return result.PasswordChangeToken
}

// enableGoogle makes the fixture's Google provider return the invitee.
func (f *invitedFixture) enableGoogle(t *testing.T, email string) {
	t.Helper()
	f.service.google.factory = func(string, string, string) OAuthProvider {
		return authTestOAuth{user: User{Email: email, Sub: initialGoogleSub}}
	}
	if err := f.service.ConfigureGoogleOAuth(context.Background(), OAuthConfig{GoogleClientID: "id", GoogleClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
}

func TestInitialPasswordChangeCompletesTheSignIn(t *testing.T) {
	f := newInvitedFixture(t)
	f.issue(t)
	ctx := context.Background()
	token := f.initialChangeToken(t)

	result, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "1.2.3.4", "browser")
	if err != nil || !result.Completed || result.CookieValue == "" || result.PasswordChangeRequired {
		t.Fatalf("CompleteInitialPasswordChange = %+v, %v", result, err)
	}
	session, err := f.service.CurrentSession(ctx, result.CookieValue)
	if err != nil || session.Email != invitedEmail || session.Sub != subjectPassword {
		t.Fatalf("session = %+v, %v", session, err)
	}
	stored := f.creds.records[invitedEmail]
	if stored.MustChangePassword || stored.Version != 2 {
		t.Fatalf("stored = %+v", stored)
	}
	if f.service.PasswordStates(ctx)[invitedEmail].ChangePending {
		t.Fatal("still pending after the change")
	}
	if _, err := f.service.LoginLocal(ctx, invitedEmail, invitedTemp); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("temporary password after the change = %v", err)
	}
	if user, err := f.service.LoginLocal(ctx, invitedEmail, initialNewSecret); err != nil || user.Sub != subjectPassword {
		t.Fatalf("new password = %+v, %v", user, err)
	}
	f.assertNoLeak(t, err)
	if strings.Contains(f.logs.String(), initialNewSecret) {
		t.Fatal("new password logged")
	}
}

func TestNoSessionExistsBeforeThePasswordIsChanged(t *testing.T) {
	ctx := context.Background()

	t.Run("password sign-in", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.issue(t)
		result, err := f.service.CompletePasswordLogin(ctx, invitedEmail, invitedTemp, "1.2.3.4", "browser")
		if err != nil || result.Completed || result.CookieValue != "" || result.PendingToken != "" {
			t.Fatalf("result = %+v, %v", result, err)
		}
		// The change token is not a session, and no sign-in was registered.
		if _, err := f.service.CurrentSession(ctx, result.PasswordChangeToken); err == nil {
			t.Fatal("password-change token accepted as a session cookie")
		}
		if got := f.registry.records[invitedEmail]; got.ActiveSessionID != "" || len(got.History.Entries) != 0 {
			t.Fatalf("a sign-in was registered: %+v", got)
		}
	})

	t.Run("google sign-in", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.issue(t)
		f.enableGoogle(t, invitedEmail)
		result, err := f.service.CompleteGoogleLogin(ctx, "code", "1.2.3.4", "browser")
		if err != nil || !result.PasswordChangeRequired || result.PasswordChangeToken == "" ||
			result.Completed || result.CookieValue != "" || result.PendingToken != "" {
			t.Fatalf("result = %+v, %v", result, err)
		}
		if _, err := f.service.CurrentSession(ctx, result.PasswordChangeToken); err == nil {
			t.Fatal("password-change token accepted as a session cookie")
		}
	})
}

func TestGoogleSignInWithoutAPendingPasswordIsUnchanged(t *testing.T) {
	ctx := context.Background()
	f := newInvitedFixture(t)
	f.enableGoogle(t, "google@example.com")
	// No credential at all: a plain Google member signs straight in.
	result, err := f.service.CompleteGoogleLogin(ctx, "code", "", "")
	if err != nil || !result.Completed || result.PasswordChangeRequired {
		t.Fatalf("google-only member = %+v, %v", result, err)
	}
	// A settled credential no longer holds Google back.
	f.settle(t)
	f.enableGoogle(t, invitedEmail)
	result, err = f.service.CompleteGoogleLogin(ctx, "code", "", "")
	if err != nil || !result.Completed || result.PasswordChangeRequired {
		t.Fatalf("settled invitee = %+v, %v", result, err)
	}
}

func TestGoogleSignInChangesThePasswordAndKeepsTheGoogleIdentity(t *testing.T) {
	ctx := context.Background()
	f := newInvitedFixture(t)
	// D7: a Google-only member gains a credential through a reset.
	pw, err := f.service.IssueTemporaryPassword(ctx, "google@example.com")
	if err != nil || pw != invitedTemp {
		t.Fatal(pw, err)
	}
	f.enableGoogle(t, "google@example.com")
	login, err := f.service.CompleteGoogleLogin(ctx, "code", "", "")
	if err != nil || !login.PasswordChangeRequired {
		t.Fatalf("login = %+v, %v", login, err)
	}
	result, err := f.service.CompleteInitialPasswordChange(ctx, login.PasswordChangeToken, initialNewSecret, "", "")
	if err != nil || !result.Completed {
		t.Fatalf("change = %+v, %v", result, err)
	}
	session, err := f.service.CurrentSession(ctx, result.CookieValue)
	if err != nil || session.Sub != initialGoogleSub || session.Email != "google@example.com" {
		t.Fatalf("session = %+v, %v", session, err)
	}
	if _, err := f.service.CompleteInitialPasswordChange(ctx, login.PasswordChangeToken, "another new password", "", ""); !errors.Is(err, ErrInvalidPasswordChange) {
		t.Fatalf("replay = %v", err)
	}
}

func TestInitialPasswordChangeRejectsTheTemporaryPasswordAndWeakOnes(t *testing.T) {
	f := newInvitedFixture(t)
	f.issue(t)
	ctx := context.Background()
	token := f.initialChangeToken(t)
	before := f.creds.records[invitedEmail]

	for name, tc := range map[string]struct {
		password string
		want     error
	}{
		"temporary password": {invitedTemp, ErrPasswordReuse},
		"too short":          {"short", ErrPasswordTooShort},
		"empty":              {"", ErrPasswordTooShort},
		"too long":           {strings.Repeat("x", passwordMaxBytes+1), ErrPasswordTooLong},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := f.service.CompleteInitialPasswordChange(ctx, token, tc.password, "", "")
			if !errors.Is(err, tc.want) || result.Completed || result.CookieValue != "" {
				t.Fatalf("result = %+v, %v; want %v", result, err, tc.want)
			}
			if f.creds.records[invitedEmail] != before {
				t.Fatal("a rejected password changed the credential")
			}
			f.assertNoLeak(t, err)
		})
	}
	// The token survives a rejected password, so the owner can retry.
	if result, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", ""); err != nil || !result.Completed {
		t.Fatalf("retry = %+v, %v", result, err)
	}
}

func TestPasswordChangeTokenIsRejectedWhenInvalid(t *testing.T) {
	f := newInvitedFixture(t)
	f.issue(t)
	ctx := context.Background()
	token := f.initialChangeToken(t)
	account := f.creds.records[invitedEmail]
	valid := pendingPasswordChange{
		Email: invitedEmail, Sub: subjectPassword, Method: SignInMethodPassword,
		Version: account.Version, Fingerprint: passwordHashFingerprint(account.PasswordHash),
		Exp: time.Now().Add(time.Minute).Unix(),
	}
	codec := f.service.passwordChangeCodec

	expired := valid
	expired.Exp = time.Now().Add(-time.Second).Unix()
	otherVersion := valid
	otherVersion.Version++
	otherHash := valid
	otherHash.Fingerprint = passwordHashFingerprint("another hash")
	otherAccount := valid
	otherAccount.Email = "google@example.com"
	localAdmin := valid
	localAdmin.Email = "admin@example.com"
	foreignKey := newPasswordChangePayload([]byte("another key"))

	// A token minted for another purpose with the same key and the same JSON
	// fields must not verify as a password change.
	pending := f.service.pendingLoginCodec.sign(pendingLogin{
		Email: invitedEmail, Sub: subjectPassword, Method: SignInMethodPassword, Exp: valid.Exp,
	})
	session := f.service.codec.sign(User{Email: invitedEmail, Sub: subjectPassword}, "")

	tampered := []byte(token)
	tampered[len(tampered)-2] ^= 1

	for name, raw := range map[string]string{
		"empty":                 "",
		"garbage":               "not a token",
		"tampered signature":    string(tampered),
		"truncated":             token[:len(token)/2],
		"expired":               codec.sign(expired),
		"newer version":         codec.sign(otherVersion),
		"other password hash":   codec.sign(otherHash),
		"account with no pw":    codec.sign(otherAccount),
		"local administrator":   codec.sign(localAdmin),
		"another signing key":   foreignKey.sign(valid),
		"pending 2FA login":     pending,
		"session cookie":        session,
		"password-change twice": token + "." + token,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := f.service.CompleteInitialPasswordChange(ctx, raw, initialNewSecret, "", "")
			if !errors.Is(err, ErrInvalidPasswordChange) || result.Completed || result.CookieValue != "" {
				t.Fatalf("result = %+v, %v", result, err)
			}
		})
	}
	// None of that touched the credential, and the real token still works.
	if !f.creds.records[invitedEmail].MustChangePassword {
		t.Fatal("rejected tokens changed the credential")
	}
	if result, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", ""); err != nil || !result.Completed {
		t.Fatalf("real token = %+v, %v", result, err)
	}
	// A password-change token is not a session either.
	if _, err := f.service.CurrentSession(ctx, token); err == nil {
		t.Fatal("password-change token accepted as a session cookie")
	}
	if _, err := f.service.CompleteTwoFactorChallenge(ctx, token, "000000", "", ""); !errors.Is(err, ErrInvalidPendingLogin) {
		t.Fatalf("password-change token accepted as a pending login: %v", err)
	}
}

func TestPasswordChangeTokenDiesWithAResetOrAChange(t *testing.T) {
	ctx := context.Background()

	t.Run("reset", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.issue(t)
		token := f.initialChangeToken(t)
		f.service.temporaryPassword = func() (string, error) { return "SECND-PASSW-ORD22-ZYXWV", nil }
		if _, err := f.service.IssueTemporaryPassword(ctx, invitedEmail); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", ""); !errors.Is(err, ErrInvalidPasswordChange) {
			t.Fatalf("token after a reset = %v", err)
		}
		if !f.creds.records[invitedEmail].MustChangePassword {
			t.Fatal("a stale token changed the credential")
		}
	})

	t.Run("change", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.issue(t)
		token := f.initialChangeToken(t)
		other := f.initialChangeToken(t)
		if _, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", ""); err != nil {
			t.Fatal(err)
		}
		// Both a replay and a second token minted before the change are dead.
		for _, stale := range []string{token, other} {
			if _, err := f.service.CompleteInitialPasswordChange(ctx, stale, "yet another password", "", ""); !errors.Is(err, ErrInvalidPasswordChange) {
				t.Fatalf("token after a change = %v", err)
			}
		}
		if _, err := f.service.LoginLocal(ctx, invitedEmail, initialNewSecret); err != nil {
			t.Fatalf("first change was overwritten: %v", err)
		}
	})

	t.Run("self-service change", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.issue(t)
		token := f.initialChangeToken(t)
		if _, err := f.service.ChangePassword(ctx, invitedUser(invitedEmail),
			PasswordChange{Current: invitedTemp, New: invitedChosen}, "", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", ""); !errors.Is(err, ErrInvalidPasswordChange) {
			t.Fatalf("token after ChangePassword = %v", err)
		}
	})
}

// A removed account loses its credential and, with it, the version counter, so
// a re-invite restarts at version 1. The token minted before the removal must
// still be dead.
func TestPasswordChangeTokenDoesNotSurviveARemovalAndReinvite(t *testing.T) {
	f := newInvitedFixture(t)
	f.issue(t)
	ctx := context.Background()
	token := f.initialChangeToken(t)
	if f.creds.records[invitedEmail].Version != 1 {
		t.Fatal("setup: expected version 1")
	}

	// Removal as the cleanup performs it, and a process restart that forgets
	// the cached counter.
	delete(f.creds.records, invitedEmail)
	f.service.accounts.mu.Lock()
	delete(f.service.accounts.cache, invitedEmail)
	f.service.accounts.mu.Unlock()
	f.issue(t)
	if got := f.creds.records[invitedEmail].Version; got != 1 {
		t.Fatalf("re-invite version = %d, want 1 (the collision under test)", got)
	}

	if _, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", ""); !errors.Is(err, ErrInvalidPasswordChange) {
		t.Fatalf("token from before the removal = %v", err)
	}
	// The new incarnation's own token works.
	fresh := f.initialChangeToken(t)
	if _, err := f.service.CompleteInitialPasswordChange(ctx, fresh, initialNewSecret, "", ""); err != nil {
		t.Fatalf("fresh token = %v", err)
	}
}

func TestInitialPasswordChangeRefusesAnUnregisteredOrSettledAccount(t *testing.T) {
	ctx := context.Background()

	t.Run("removed user", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.issue(t)
		token := f.initialChangeToken(t)
		delete(f.users.roles, invitedEmail)
		if _, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", ""); !errors.Is(err, ErrInvalidPasswordChange) {
			t.Fatalf("removed user = %v", err)
		}
	})

	t.Run("directory failure", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.issue(t)
		token := f.initialChangeToken(t)
		f.users.isRegisteredErr = errors.New("directory down")
		_, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", "")
		if !errors.Is(err, ErrPasswordNotChanged) {
			t.Fatalf("directory failure = %v", err)
		}
		f.users.isRegisteredErr = nil
		if _, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", ""); err != nil {
			t.Fatalf("retry after the directory recovered = %v", err)
		}
	})

	t.Run("settled account", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.settle(t)
		account := f.creds.records[invitedEmail]
		token := f.service.passwordChangeCodec.sign(pendingPasswordChange{
			Email: invitedEmail, Sub: subjectPassword, Method: SignInMethodPassword,
			Version: account.Version, Fingerprint: passwordHashFingerprint(account.PasswordHash),
			Exp: time.Now().Add(time.Minute).Unix(),
		})
		if _, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", ""); !errors.Is(err, ErrInvalidPasswordChange) {
			t.Fatalf("settled account = %v", err)
		}
	})
}

func TestInitialPasswordChangeHandsOffToTwoFactor(t *testing.T) {
	f := newInvitedFixture(t)
	f.issue(t)
	ctx := context.Background()
	secret, _ := enrollServiceAccount(t, f.service, invitedEmail)
	token := f.initialChangeToken(t)

	result, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "1.2.3.4", "browser")
	if err != nil || result.Completed || result.CookieValue != "" || result.PendingToken == "" || result.PasswordChangeRequired {
		t.Fatalf("change = %+v, %v", result, err)
	}
	if _, err := f.service.CurrentSession(ctx, result.PendingToken); err == nil {
		t.Fatal("pending 2FA token accepted as a session")
	}
	if _, err := f.service.CompleteTwoFactorChallenge(ctx, result.PendingToken, "000000", "", ""); err == nil {
		t.Fatal("challenge completed with a wrong code")
	}
	done, err := f.service.CompleteTwoFactorChallenge(ctx, result.PendingToken, TOTPCode(secret, time.Now()), "1.2.3.4", "browser")
	if err != nil || !done.Completed {
		t.Fatalf("challenge = %+v, %v", done, err)
	}
	session, err := f.service.CurrentSession(ctx, done.CookieValue)
	if err != nil || session.Sub != subjectPassword {
		t.Fatalf("session = %+v, %v", session, err)
	}
	// The password is already changed: the temporary one is gone for good.
	if _, err := f.service.LoginLocal(ctx, invitedEmail, invitedTemp); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("temporary password after the hand-off = %v", err)
	}
}

func TestInitialPasswordChangeKeepsTheGoogleMethodThroughTwoFactor(t *testing.T) {
	f := newInvitedFixture(t)
	ctx := context.Background()
	if _, err := f.service.IssueTemporaryPassword(ctx, "google@example.com"); err != nil {
		t.Fatal(err)
	}
	secret, _ := enrollServiceAccount(t, f.service, "google@example.com")
	f.enableGoogle(t, "google@example.com")
	if err := f.service.SetSecurityPreferences(ctx, "google@example.com", SecurityPreferences{HistoryEnabled: true}); err != nil {
		t.Fatal(err)
	}
	login, err := f.service.CompleteGoogleLogin(ctx, "code", "", "")
	if err != nil || !login.PasswordChangeRequired {
		t.Fatalf("login = %+v, %v", login, err)
	}
	pending, err := f.service.CompleteInitialPasswordChange(ctx, login.PasswordChangeToken, initialNewSecret, "", "")
	if err != nil || pending.PendingToken == "" {
		t.Fatalf("change = %+v, %v", pending, err)
	}
	if _, err := f.service.CompleteTwoFactorChallenge(ctx, pending.PendingToken, TOTPCode(secret, time.Now()), "", ""); err != nil {
		t.Fatal(err)
	}
	history, err := f.service.registry.History(ctx, "google@example.com")
	if err != nil || len(history.Entries) != 1 || history.Entries[0].Method != SignInMethodGoogleTOTP {
		t.Fatalf("history = %+v, %v", history, err)
	}
}

func TestInitialPasswordChangeRevokesBeforeSaving(t *testing.T) {
	f := newInvitedFixture(t)
	f.issue(t)
	ctx := context.Background()
	token := f.initialChangeToken(t)
	// Revoking in the same second as the issue is a no-op; start clean so the
	// change's own revocation reaches the registry.
	f.forgetRevocations()

	var revokedAtSave int64
	f.creds.beforeSave = func(string, AccountCredential) {
		revokedAtSave = f.registry.records[invitedEmail].SessionsValidAfter
	}
	if _, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", ""); err != nil {
		t.Fatal(err)
	}
	if revokedAtSave == 0 {
		t.Fatal("sessions were not revoked before the new credential was saved")
	}
}

func TestInitialPasswordChangeFailurePaths(t *testing.T) {
	ctx := context.Background()

	t.Run("revocation fails and nothing is saved", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.issue(t)
		token := f.initialChangeToken(t)
		f.forgetRevocations()
		failing := &failingRegistryStore{authTestSessionRegistryStore: f.registry, err: errors.New("disk full")}
		f.service.registry = newSessionRegistry(failing, 20)
		before := f.creds.records[invitedEmail]
		saves := f.creds.saveCalls

		result, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", "")
		if !errors.Is(err, ErrPasswordNotChanged) || result.Completed {
			t.Fatalf("result = %+v, %v", result, err)
		}
		if f.creds.records[invitedEmail] != before || f.creds.saveCalls != saves {
			t.Fatal("the credential was written although revocation failed")
		}
		f.assertNoLeak(t, err)
		// Retry succeeds once the registry recovers.
		failing.err = nil
		if _, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", ""); err != nil {
			t.Fatalf("retry = %v", err)
		}
	})

	t.Run("save fails", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.issue(t)
		token := f.initialChangeToken(t)
		before := f.creds.records[invitedEmail]
		f.creds.saveErr = errors.New("disk full")

		result, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", "")
		if !errors.Is(err, ErrPasswordNotChangedSessionsRevoked) || result.Completed || result.CookieValue != "" {
			t.Fatalf("result = %+v, %v", result, err)
		}
		if f.creds.records[invitedEmail] != before || !f.service.PasswordStates(ctx)[invitedEmail].ChangePending {
			t.Fatal("a failed save changed the credential")
		}
		f.assertNoLeak(t, err)
		f.creds.saveErr = nil
		if _, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", ""); err != nil {
			t.Fatalf("retry = %v", err)
		}
	})

	t.Run("session issue fails after the change", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.issue(t)
		ctx := context.Background()
		if err := f.service.SetSecurityPreferences(ctx, invitedEmail, SecurityPreferences{HistoryEnabled: true}); err != nil {
			t.Fatal(err)
		}
		token := f.initialChangeToken(t)
		// Only recording the sign-in fails; the revocation before it succeeds.
		f.service.registry = newSessionRegistry(&failingRegistryStore{
			authTestSessionRegistryStore: f.registry, err: errors.New("disk full"), onlyHistory: true,
		}, 20)

		_, err := f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", "")
		if !errors.Is(err, ErrPasswordChangedSessionFailed) {
			t.Fatalf("err = %v", err)
		}
		// The password did change: signing in with the new one is the way on.
		if _, err := f.service.LoginLocal(ctx, invitedEmail, initialNewSecret); err != nil {
			t.Fatalf("new password after a failed session issue = %v", err)
		}
	})
}

// failingRegistryStore fails Save with err, or only a Save that records a
// sign-in in the history when onlyHistory is set.
type failingRegistryStore struct {
	*authTestSessionRegistryStore
	err         error
	onlyHistory bool
}

func (s *failingRegistryStore) Save(ctx context.Context, email string, record SessionRegistryRecord) error {
	if s.err != nil && (!s.onlyHistory || len(record.History.Entries) > 0) {
		return s.err
	}
	return s.authTestSessionRegistryStore.Save(ctx, email, record)
}

func TestConcurrentInitialPasswordChangesSaveExactlyOne(t *testing.T) {
	f := newInvitedFixture(t)
	f.issue(t)
	ctx := context.Background()
	token := f.initialChangeToken(t)

	passwords := []string{"first new password!", "second new password", "third new password!"}
	errs := make([]error, len(passwords))
	var wg sync.WaitGroup
	for i, password := range passwords {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.service.CompleteInitialPasswordChange(ctx, token, password, "", "")
		}()
	}
	wg.Wait()

	winner := -1
	for i, err := range errs {
		switch {
		case err == nil:
			if winner != -1 {
				t.Fatalf("two changes succeeded: %v", errs)
			}
			winner = i
		case !errors.Is(err, ErrInvalidPasswordChange):
			t.Fatalf("loser %d = %v", i, err)
		}
	}
	if winner == -1 {
		t.Fatalf("no change succeeded: %v", errs)
	}
	if got := f.creds.records[invitedEmail].Version; got != 2 {
		t.Fatalf("version = %d, want 2", got)
	}
	for i, password := range passwords {
		_, err := f.service.LoginLocal(ctx, invitedEmail, password)
		if (err == nil) != (i == winner) {
			t.Fatalf("password %d login = %v (winner %d)", i, err, winner)
		}
	}
}

func TestResetRacingAChangeNeverLetsTheStaleTokenWin(t *testing.T) {
	f := newInvitedFixture(t)
	f.issue(t)
	ctx := context.Background()
	token := f.initialChangeToken(t)
	f.service.temporaryPassword = func() (string, error) { return "SECND-PASSW-ORD22-ZYXWV", nil }

	var wg sync.WaitGroup
	var changeErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, changeErr = f.service.CompleteInitialPasswordChange(ctx, token, initialNewSecret, "", "")
	}()
	go func() {
		defer wg.Done()
		if _, err := f.service.IssueTemporaryPassword(ctx, invitedEmail); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()

	// Whichever order the lock gave, the reset's password must be the last
	// word: either the change ran first and the reset replaced it, or the
	// reset ran first and the change was refused.
	if changeErr != nil && !errors.Is(changeErr, ErrInvalidPasswordChange) {
		t.Fatalf("change = %v", changeErr)
	}
	if _, err := f.service.LoginLocal(ctx, invitedEmail, "SECND-PASSW-ORD22-ZYXWV"); !errors.Is(err, ErrPasswordChangeRequired) {
		t.Fatalf("reset password after the race = %v", err)
	}
	if _, err := f.service.LoginLocal(ctx, invitedEmail, initialNewSecret); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("chosen password survived the reset: %v", err)
	}
}

func TestPasswordChangeDuration(t *testing.T) {
	f := newInvitedFixture(t)
	if got := f.service.PasswordChangeDuration(); got != 15*time.Minute {
		t.Fatalf("PasswordChangeDuration = %v", got)
	}
}
