package auth

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	invitedEmail    = "invitee@example.com"
	invitedTemp     = "TEMP7-PASSW-ORD22-ABCDE"
	invitedChosen   = "a freshly chosen password"
	invitedLocalPwd = "local admin password"
)

type invitedFixture struct {
	service  *Service
	users    *authTestUsers
	creds    *authTestAccountCredentialStore
	registry *authTestSessionRegistryStore
	verifies atomic.Int64
	logs     *bytes.Buffer
}

// newInvitedFixture builds a service with a local administrator, one
// registered Google-only member and one registered invitee, counting every
// password verification and capturing everything logged through slog.
func newInvitedFixture(t *testing.T) *invitedFixture {
	t.Helper()
	f := &invitedFixture{
		users:    newAuthTestUsers(),
		creds:    newAuthTestAccountCredentialStore(),
		registry: newAuthTestSessionRegistryStore(),
		logs:     &bytes.Buffer{},
	}
	hash, err := HashPassword(invitedLocalPwd)
	if err != nil {
		t.Fatal(err)
	}
	store := &authTestStore{local: &LocalAdminCredential{Email: "admin@example.com", PasswordHash: hash}, key: []byte("k")}
	f.users.roles["admin@example.com"] = true
	f.users.roles[invitedEmail] = false
	f.users.roles["google@example.com"] = false
	service, err := New(
		context.Background(), store, f.users,
		func(string, string, string) OAuthProvider { return authTestOAuth{} },
		"https://remote.example.com", []byte("test-session-key"),
		newAuthTestTwoFactorStore(), f.registry, f.creds, authTestOptions(),
	)
	if err != nil {
		t.Fatal(err)
	}
	service.verifyPassword = func(encoded, password string) (bool, error) {
		f.verifies.Add(1)
		return VerifyPassword(encoded, password)
	}
	service.temporaryPassword = func() (string, error) { return invitedTemp, nil }
	f.service = service

	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return f
}

func (f *invitedFixture) issue(t *testing.T) {
	t.Helper()
	got, err := f.service.IssueTemporaryPassword(context.Background(), invitedEmail)
	if err != nil || got != invitedTemp {
		t.Fatalf("IssueTemporaryPassword = %q, %v", got, err)
	}
}

// settle gives the invitee a password of their own choosing.
func (f *invitedFixture) settle(t *testing.T) {
	t.Helper()
	f.issue(t)
	if _, err := f.service.ChangePassword(context.Background(), invitedUser(invitedEmail),
		PasswordChange{Current: invitedTemp, New: invitedChosen}, "", ""); err != nil {
		t.Fatal(err)
	}
	f.verifies.Store(0)
}

// forgetRevocations drops the account's session-registry record. Revoking at
// the same second as an earlier revocation is a no-op, so a test that wants
// the next revocation to reach the store (to fail or observe it) starts clean.
func (f *invitedFixture) forgetRevocations() {
	delete(f.registry.records, invitedEmail)
	f.service.registry.setCache(invitedEmail, nil)
}

func (f *invitedFixture) assertNoLeak(t *testing.T, errs ...error) {
	t.Helper()
	for _, secret := range []string{invitedTemp, invitedChosen} {
		if strings.Contains(f.logs.String(), secret) {
			t.Fatalf("password leaked to logs: %s", f.logs.String())
		}
		for _, err := range errs {
			if err != nil && strings.Contains(err.Error(), secret) {
				t.Fatalf("password leaked in error: %v", err)
			}
		}
		for _, stored := range f.creds.records {
			if strings.Contains(stored.PasswordHash, secret) {
				t.Fatal("plaintext persisted")
			}
		}
	}
}

func TestLoginVerifiesExactlyOncePerPath(t *testing.T) {
	f := newInvitedFixture(t)
	f.settle(t)
	// An orphan: a credential whose user was removed.
	f.creds.records["orphan@example.com"] = AccountCredential{PasswordHash: f.creds.records[invitedEmail].PasswordHash, Version: 1}
	reloadCredentials(t, f)
	// An invited credential squatting on the local administrator's email.
	f.creds.records["admin@example.com"] = f.creds.records[invitedEmail]
	reloadCredentials(t, f)

	for _, tc := range []struct {
		name, email, password string
		wantOK                bool
	}{
		{"local admin right", "admin@example.com", invitedLocalPwd, true},
		{"local admin wrong", "admin@example.com", "wrong password!!", false},
		{"invited right", invitedEmail, invitedChosen, true},
		{"invited wrong", invitedEmail, "wrong password!!", false},
		{"invited, mixed-case email", " Invitee@Example.com", invitedChosen, true},
		{"unknown email", "nobody@example.com", invitedChosen, false},
		{"orphan credential", "orphan@example.com", invitedChosen, false},
		{"registered without credential", "google@example.com", invitedChosen, false},
		{"credential squatting on the admin email", "admin@example.com", invitedChosen, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.verifies.Store(0)
			user, err := f.service.LoginLocal(context.Background(), tc.email, tc.password)
			if got := f.verifies.Load(); got != 1 {
				t.Fatalf("%d password verifications, want exactly 1", got)
			}
			if tc.wantOK && err != nil {
				t.Fatalf("login = %v", err)
			}
			if !tc.wantOK && !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("login = %+v, %v; want ErrInvalidCredentials", user, err)
			}
		})
	}
}

// reloadCredentials reloads the credential cache from the store, as a restart
// would.
func reloadCredentials(t *testing.T, f *invitedFixture) {
	t.Helper()
	accounts, err := newAccountPasswordAuthenticator(context.Background(), f.creds)
	if err != nil {
		t.Fatal(err)
	}
	f.service.accounts = accounts
}

func TestLoginVerifiesOnceWithoutAnyLocalAdmin(t *testing.T) {
	f := newInvitedFixture(t)
	f.settle(t)
	f.service.local.setCredential(nil)
	for _, email := range []string{invitedEmail, "nobody@example.com"} {
		f.verifies.Store(0)
		_, err := f.service.LoginLocal(context.Background(), email, invitedChosen)
		if f.verifies.Load() != 1 {
			t.Fatalf("%s: %d verifications, want 1", email, f.verifies.Load())
		}
		if email == invitedEmail && err != nil {
			t.Fatalf("invited login without a local admin = %v", err)
		}
		if email != invitedEmail && !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("unknown login = %v", err)
		}
	}
}

func TestLoginFailsClosedWithOneVerificationWhenTheDirectoryFails(t *testing.T) {
	f := newInvitedFixture(t)
	f.settle(t)
	f.users.isRegisteredErr = errors.New("directory down")
	_, err := f.service.LoginLocal(context.Background(), invitedEmail, invitedChosen)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	if f.verifies.Load() != 1 {
		t.Fatalf("%d verifications, want 1", f.verifies.Load())
	}
}

func TestRemovedUsersCredentialIsIgnoredEvenWithTheRightPassword(t *testing.T) {
	f := newInvitedFixture(t)
	f.settle(t)
	delete(f.users.roles, invitedEmail)
	f.verifies.Store(0)
	if _, err := f.service.LoginLocal(context.Background(), invitedEmail, invitedChosen); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login = %v", err)
	}
	if f.verifies.Load() != 1 {
		t.Fatalf("%d verifications, want 1", f.verifies.Load())
	}
	// Changing the password of a removed account is refused the same way.
	if _, err := f.service.ChangePassword(context.Background(), invitedUser(invitedEmail),
		PasswordChange{Current: invitedChosen, New: "another password!!"}, "", ""); !errors.Is(err, ErrPasswordSignInUnavailable) {
		t.Fatalf("ChangePassword for a removed user = %v", err)
	}
}

func TestTemporaryPasswordBlocksSignInUntilChanged(t *testing.T) {
	f := newInvitedFixture(t)
	f.issue(t)
	ctx := context.Background()

	if _, err := f.service.LoginLocal(ctx, invitedEmail, "wrong password!!"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password = %v", err)
	}
	// A correct temporary password proves the owner but gets no session.
	if _, err := f.service.LoginLocal(ctx, invitedEmail, invitedTemp); !errors.Is(err, ErrPasswordChangeRequired) {
		t.Fatalf("LoginLocal = %v, want ErrPasswordChangeRequired", err)
	}
	result, err := f.service.CompletePasswordLogin(ctx, invitedEmail, invitedTemp, "", "")
	if err != nil || !result.PasswordChangeRequired || result.PasswordChangeToken == "" ||
		result.Completed || result.CookieValue != "" || result.PendingToken != "" {
		t.Fatalf("CompletePasswordLogin = %+v, %v", result, err)
	}
	if len(f.registry.records) != 1 {
		// Only the revocation of IssueTemporaryPassword; no sign-in registered.
		t.Fatalf("registry = %+v", f.registry.records)
	}
	f.assertNoLeak(t, err)
}

func TestInvitedSignInIsRecordedAsPasswordNotGoogle(t *testing.T) {
	f := newInvitedFixture(t)
	f.settle(t)
	ctx := context.Background()
	if err := f.service.SetSecurityPreferences(ctx, invitedEmail, SecurityPreferences{HistoryEnabled: true}); err != nil {
		t.Fatal(err)
	}
	result, err := f.service.CompletePasswordLogin(ctx, invitedEmail, invitedChosen, "1.2.3.4", "browser")
	if err != nil || !result.Completed {
		t.Fatalf("login = %+v, %v", result, err)
	}
	session, err := f.service.CurrentSession(ctx, result.CookieValue)
	if err != nil || session.Sub != subjectPassword || session.Email != invitedEmail {
		t.Fatalf("session = %+v, %v", session, err)
	}
	if _, err := f.service.ReissueTrackedSession(ctx, User{Email: invitedEmail, Sub: subjectPassword}, "1.2.3.4", "browser"); err != nil {
		t.Fatal(err)
	}
	history, err := f.service.registry.History(ctx, invitedEmail)
	if err != nil || len(history.Entries) != 2 {
		t.Fatalf("history = %+v, %v", history, err)
	}
	for _, entry := range history.Entries {
		if entry.Method != SignInMethodPassword {
			t.Fatalf("recorded method %q, want password", entry.Method)
		}
	}
}

func TestIssueTemporaryPasswordIncrementsVersionAndStoresOnlyAHash(t *testing.T) {
	f := newInvitedFixture(t)
	ctx := context.Background()
	f.issue(t)
	first := f.creds.records[invitedEmail]
	if first.Version != 1 || !first.MustChangePassword || first.UpdatedAt.IsZero() {
		t.Fatalf("first = %+v", first)
	}
	if ok, err := VerifyPassword(first.PasswordHash, invitedTemp); err != nil || !ok {
		t.Fatalf("stored hash does not verify the temporary password: %v", err)
	}

	f.service.temporaryPassword = func() (string, error) { return "SECND-PASSW-ORD22-ZYXWV", nil }
	second, err := f.service.IssueTemporaryPassword(ctx, " Invitee@Example.com ")
	if err != nil {
		t.Fatal(err)
	}
	stored := f.creds.records[invitedEmail]
	if stored.Version != 2 || !stored.MustChangePassword || stored.PasswordHash == first.PasswordHash {
		t.Fatalf("second = %+v", stored)
	}
	// A reset invalidates the previous temporary password.
	if _, err := f.service.LoginLocal(ctx, invitedEmail, invitedTemp); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old temporary password = %v", err)
	}
	if _, err := f.service.LoginLocal(ctx, invitedEmail, second); !errors.Is(err, ErrPasswordChangeRequired) {
		t.Fatalf("new temporary password = %v", err)
	}
	if states := f.service.PasswordStates(ctx); len(states) != 1 || !states[invitedEmail].ChangePending {
		t.Fatalf("states = %+v", states)
	}
	f.assertNoLeak(t)
}

func TestIssueTemporaryPasswordResetsAGoogleOnlyMemberAndASettledInvitee(t *testing.T) {
	f := newInvitedFixture(t)
	ctx := context.Background()
	pw, err := f.service.IssueTemporaryPassword(ctx, "google@example.com")
	if err != nil || pw != invitedTemp {
		t.Fatalf("Google-only member = %q, %v", pw, err)
	}
	f.settle(t)
	if f.service.PasswordStates(ctx)[invitedEmail].ChangePending {
		t.Fatal("settled invitee still pending")
	}
	f.issue(t)
	if !f.service.PasswordStates(ctx)[invitedEmail].ChangePending {
		t.Fatal("reset did not force a change")
	}
	if got := f.creds.records[invitedEmail].Version; got != 3 {
		t.Fatalf("version = %d, want 3 (issue, change, reset)", got)
	}
}

func TestIssueTemporaryPasswordRevokesEarlierSessionsBeforeSaving(t *testing.T) {
	f := newInvitedFixture(t)
	ctx := context.Background()
	old := f.service.codec.payload.sign(Session{
		Email: invitedEmail, Sub: subjectPassword,
		Iat: time.Now().Add(-time.Minute).Unix(), Exp: time.Now().Add(time.Hour).Unix(),
	})
	if _, err := f.service.CurrentSession(ctx, old); err != nil {
		t.Fatalf("session invalid before the reset: %v", err)
	}
	f.creds.beforeSave = func(string, AccountCredential) {
		if _, err := f.service.CurrentSession(ctx, old); !errors.Is(err, ErrSessionRevoked) {
			t.Errorf("sessions were not revoked before the credential was saved: %v", err)
		}
	}
	f.issue(t)
	if _, err := f.service.CurrentSession(ctx, old); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("old session after reset = %v", err)
	}
}

func TestIssueTemporaryPasswordFailures(t *testing.T) {
	ctx := context.Background()
	t.Run("session revocation fails", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.settle(t)
		before := f.creds.records[invitedEmail]
		f.forgetRevocations()
		f.service.registry.store = failingSessionRegistrySaveStore{f.registry, errors.New("registry down")}
		pw, err := f.service.IssueTemporaryPassword(ctx, invitedEmail)
		if err == nil || pw != "" {
			t.Fatalf("= %q, %v", pw, err)
		}
		if f.creds.records[invitedEmail] != before {
			t.Fatal("credential changed although sessions could not be revoked")
		}
		f.assertNoLeak(t, err)
	})
	t.Run("credential save fails", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.creds.saveErr = errors.New("disk full")
		pw, err := f.service.IssueTemporaryPassword(ctx, invitedEmail)
		if !errors.Is(err, f.creds.saveErr) || pw != "" {
			t.Fatalf("= %q, %v", pw, err)
		}
		if _, ok := f.creds.records[invitedEmail]; ok {
			t.Fatal("credential saved despite the failure")
		}
		if _, ok := f.service.accounts.credential(invitedEmail); ok {
			t.Fatal("cache holds a credential the store rejected")
		}
		f.assertNoLeak(t, err)
	})
	t.Run("generator fails", func(t *testing.T) {
		f := newInvitedFixture(t)
		f.service.temporaryPassword = func() (string, error) { return "", errors.New("no entropy") }
		if pw, err := f.service.IssueTemporaryPassword(ctx, invitedEmail); err == nil || pw != "" {
			t.Fatalf("= %q, %v", pw, err)
		}
		if len(f.creds.records) != 0 || len(f.registry.records) != 0 {
			t.Fatal("state changed")
		}
	})
	t.Run("local administrator", func(t *testing.T) {
		f := newInvitedFixture(t)
		if _, err := f.service.IssueTemporaryPassword(ctx, " Admin@example.com"); !errors.Is(err, ErrTemporaryPasswordLocalAdmin) {
			t.Fatalf("err = %v", err)
		}
		if len(f.creds.records) != 0 || len(f.registry.records) != 0 {
			t.Fatal("state changed")
		}
	})
	t.Run("empty email", func(t *testing.T) {
		f := newInvitedFixture(t)
		if _, err := f.service.IssueTemporaryPassword(ctx, "  "); err == nil {
			t.Fatal("accepted")
		}
	})
}

type failingSessionRegistrySaveStore struct {
	SessionRegistryStore
	err error
}

func (s failingSessionRegistrySaveStore) Save(context.Context, string, SessionRegistryRecord) error {
	return s.err
}

func TestInvitedUserCanChangeTheirOwnPassword(t *testing.T) {
	f := newInvitedFixture(t)
	ctx := context.Background()
	f.settle(t)
	if got := f.creds.records[invitedEmail]; got.Version != 2 || got.MustChangePassword {
		t.Fatalf("after the first change = %+v", got)
	}
	old := f.service.codec.payload.sign(Session{
		Email: invitedEmail, Sub: subjectPassword,
		Iat: time.Now().Add(-time.Minute).Unix(), Exp: time.Now().Add(time.Hour).Unix(),
	})

	const second = "yet another password"
	cookie, err := f.service.ChangePassword(ctx, User{Email: " Invitee@Example.com", Sub: "google-sub"},
		PasswordChange{Current: invitedChosen, New: second}, "9.9.9.9", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if got := f.creds.records[invitedEmail]; got.Version != 3 || got.MustChangePassword {
		t.Fatalf("credential = %+v, want version 3", got)
	}
	if _, err := f.service.CurrentSession(ctx, old); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("old session = %v", err)
	}
	session, err := f.service.CurrentSession(ctx, cookie)
	if err != nil || session.Email != invitedEmail || session.Sub != subjectPassword {
		t.Fatalf("new session = %+v, %v", session, err)
	}
	if _, err := f.service.LoginLocal(ctx, invitedEmail, invitedChosen); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("previous password still works: %v", err)
	}
	if _, err := f.service.LoginLocal(ctx, invitedEmail, second); err != nil {
		t.Fatalf("new password: %v", err)
	}
	summary, err := f.service.SecuritySummary(ctx, invitedEmail)
	if err != nil || !summary.PasswordSignIn {
		t.Fatalf("summary = %+v, %v", summary, err)
	}
	if g, _ := f.service.SecuritySummary(ctx, "google@example.com"); g.PasswordSignIn {
		t.Fatal("Google-only member reports a password")
	}
	f.assertNoLeak(t)
}

func TestInvitedChangePasswordFailures(t *testing.T) {
	ctx := context.Background()
	change := func(current, next string) PasswordChange { return PasswordChange{Current: current, New: next} }
	for _, tc := range []struct {
		name      string
		setup     func(*testing.T, *invitedFixture)
		email     string
		change    PasswordChange
		want      error
		unchanged bool // credential identical afterwards
	}{
		{"wrong current", nil, invitedEmail, change("not the password", invitedChosen+"2"), ErrCurrentPasswordIncorrect, true},
		{"reuse", nil, invitedEmail, change(invitedChosen, invitedChosen), ErrPasswordReuse, true},
		{"too short", nil, invitedEmail, change(invitedChosen, "short"), ErrPasswordTooShort, true},
		{"google-only member", nil, "google@example.com", change(invitedChosen, invitedChosen+"2"), ErrPasswordSignInUnavailable, true},
		{"unknown email", nil, "nobody@example.com", change(invitedChosen, invitedChosen+"2"), ErrPasswordSignInUnavailable, true},
		{"directory failure", func(_ *testing.T, f *invitedFixture) { f.users.isRegisteredErr = errors.New("down") }, invitedEmail, change(invitedChosen, invitedChosen+"2"), ErrPasswordNotChanged, true},
		{"credential save fails", func(_ *testing.T, f *invitedFixture) { f.creds.saveErr = errors.New("disk full") }, invitedEmail, change(invitedChosen, invitedChosen+"2"), ErrPasswordNotChangedSessionsRevoked, true},
		{"revocation fails", func(_ *testing.T, f *invitedFixture) {
			f.forgetRevocations()
			f.service.registry.store = failingSessionRegistrySaveStore{f.registry, errors.New("registry down")}
		}, invitedEmail, change(invitedChosen, invitedChosen+"2"), ErrPasswordNotChanged, true},
		{"changed elsewhere meanwhile", func(t *testing.T, f *invitedFixture) {
			// Another writer bumps the version after ChangePassword verified
			// the password but before it saves.
			f.forgetRevocations()
			f.service.registry.store = hookedSessionRegistryStore{f.registry, func() {
				if err := f.service.accounts.issueLocked(ctx, invitedEmail, "intruder-hash"); err != nil {
					t.Error(err)
				}
			}}
		}, invitedEmail, change(invitedChosen, invitedChosen+"2"), ErrAccountCredentialChanged, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newInvitedFixture(t)
			f.settle(t)
			if tc.setup != nil {
				tc.setup(t, f)
			}
			before := f.creds.records[invitedEmail]
			cookie, err := f.service.ChangePassword(ctx, invitedUser(tc.email), tc.change, "", "")
			if !errors.Is(err, tc.want) || cookie != "" {
				t.Fatalf("= %q, %v; want %v", cookie, err, tc.want)
			}
			if tc.unchanged && f.creds.records[invitedEmail] != before {
				t.Fatalf("credential changed: %+v -> %+v", before, f.creds.records[invitedEmail])
			}
			if !tc.unchanged && f.creds.records[invitedEmail].PasswordHash != "intruder-hash" {
				t.Fatal("the stale change overwrote the newer credential")
			}
			f.assertNoLeak(t, err)
		})
	}
}

type hookedSessionRegistryStore struct {
	SessionRegistryStore
	onSave func()
}

func (s hookedSessionRegistryStore) Save(ctx context.Context, email string, record SessionRegistryRecord) error {
	s.onSave()
	return s.SessionRegistryStore.Save(ctx, email, record)
}

func TestInvitedChangePasswordRequiresTwoFactorWhenEnabled(t *testing.T) {
	f := newInvitedFixture(t)
	ctx := context.Background()
	f.settle(t)
	_, codes := enrollServiceAccount(t, f.service, invitedEmail)

	_, err := f.service.ChangePassword(ctx, invitedUser(invitedEmail), PasswordChange{Current: invitedChosen, New: invitedChosen + "2"}, "", "")
	if !errors.Is(err, ErrTwoFactorCodeRequired) {
		t.Fatalf("without a code = %v", err)
	}
	if got := f.creds.records[invitedEmail].Version; got != 2 {
		t.Fatalf("version = %d", got)
	}
	if _, err := f.service.ChangePassword(ctx, invitedUser(invitedEmail), PasswordChange{Current: invitedChosen, New: invitedChosen + "2", Code: codes[0]}, "", ""); err != nil {
		t.Fatalf("with a recovery code = %v", err)
	}
}

func TestConcurrentIssueAndChangeNeverInterleave(t *testing.T) {
	for range 8 {
		f := newInvitedFixture(t)
		ctx := context.Background()
		f.settle(t)
		// Whatever the interleaving, the end state must match one serial order.
		var wg sync.WaitGroup
		start := make(chan struct{})
		var changeErr error
		var issueErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, changeErr = f.service.ChangePassword(ctx, invitedUser(invitedEmail), PasswordChange{Current: invitedChosen, New: invitedChosen + "2"}, "", "")
		}()
		go func() {
			defer wg.Done()
			<-start
			_, issueErr = f.service.IssueTemporaryPassword(ctx, invitedEmail)
		}()
		close(start)
		wg.Wait()
		if issueErr != nil {
			t.Fatalf("issue = %v", issueErr)
		}
		final := f.creds.records[invitedEmail]
		switch {
		case changeErr == nil:
			// change, then reset: the temporary password is what is left.
			if final.Version != 4 || !final.MustChangePassword {
				t.Fatalf("change-then-issue ended at %+v", final)
			}
			if ok, _ := VerifyPassword(final.PasswordHash, invitedTemp); !ok {
				t.Fatal("the reset's password is not the stored one")
			}
		case errors.Is(changeErr, ErrCurrentPasswordIncorrect):
			// reset, then a change with the now-dead old password.
			if final.Version != 3 || !final.MustChangePassword {
				t.Fatalf("issue-then-change ended at %+v", final)
			}
		default:
			t.Fatalf("change = %v", changeErr)
		}
		f.assertNoLeak(t, changeErr)
	}
}

func TestNewRejectsAMissingOrUnreadableCredentialStore(t *testing.T) {
	build := func(store AccountCredentialStore) error {
		_, err := New(context.Background(), &authTestStore{}, newAuthTestUsers(),
			func(string, string, string) OAuthProvider { return authTestOAuth{} },
			"https://remote.example.com", []byte("k"),
			newAuthTestTwoFactorStore(), newAuthTestSessionRegistryStore(), store, authTestOptions())
		return err
	}
	if err := build(nil); err == nil {
		t.Fatal("nil store accepted")
	}
	broken := newAuthTestAccountCredentialStore()
	broken.listErr = errors.New("unreadable")
	if err := build(broken); !errors.Is(err, broken.listErr) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteAccountCredentialRemovesItFromStoreAndMemory(t *testing.T) {
	f := newInvitedFixture(t)
	f.settle(t)
	ctx := context.Background()

	if err := f.service.DeleteAccountCredential(ctx, " Invitee@Example.com "); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.creds.records[invitedEmail]; ok {
		t.Fatal("credential left in the store")
	}
	if _, ok := f.service.accounts.credential(invitedEmail); ok {
		t.Fatal("credential left in memory")
	}
	// Still registered, as after a Google re-invite: the old password is dead.
	if _, err := f.service.LoginLocal(ctx, invitedEmail, invitedChosen); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password after the delete = %v", err)
	}
	if len(f.service.PasswordStates(ctx)) != 0 {
		t.Fatalf("states = %+v", f.service.PasswordStates(ctx))
	}
	// Idempotent.
	if err := f.service.DeleteAccountCredential(ctx, invitedEmail); err != nil {
		t.Fatal(err)
	}
}

type deleteFailingCredentialStore struct {
	*authTestAccountCredentialStore
}

func (deleteFailingCredentialStore) Delete(context.Context, string) error {
	return errors.New("disk full")
}

func TestDeleteAccountCredentialKeepsMemoryWhenTheStoreFails(t *testing.T) {
	f := newInvitedFixture(t)
	f.settle(t)
	f.service.accounts.store = deleteFailingCredentialStore{f.creds}

	if err := f.service.DeleteAccountCredential(context.Background(), invitedEmail); err == nil {
		t.Fatal("delete reported success")
	}
	if _, ok := f.service.accounts.credential(invitedEmail); !ok {
		t.Fatal("cache dropped a credential the store still holds")
	}
}
