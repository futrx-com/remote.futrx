package auth

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const changeTestEmail = "admin@example.com"
const changeTestCurrent = "current password secret"
const changeTestNew = "replacement password secret"

type passwordChangeStore struct {
	*authTestStore
	events     *[]string
	replaceErr error
}

func (s *passwordChangeStore) ReplaceLocalAdmin(ctx context.Context, expected, next LocalAdminCredential) error {
	*s.events = append(*s.events, "save")
	if s.replaceErr != nil {
		return s.replaceErr
	}
	return s.authTestStore.ReplaceLocalAdmin(ctx, expected, next)
}

type passwordChangeRegistryStore struct {
	*authTestSessionRegistryStore
	events    *[]string
	saveCalls int
	failCall  int
	failure   error
}

func (s *passwordChangeRegistryStore) Save(ctx context.Context, email string, record SessionRegistryRecord) error {
	s.saveCalls++
	event := "reissue"
	if s.saveCalls == 1 {
		event = "revoke"
	}
	*s.events = append(*s.events, event)
	if s.saveCalls == s.failCall {
		return s.failure
	}
	return s.authTestSessionRegistryStore.Save(ctx, email, record)
}

type failingPasswordChangeTwoFactorStore struct {
	TwoFactorStore
	err error
}

func (s failingPasswordChangeTwoFactorStore) Get(context.Context, string) (*TwoFactorRecord, error) {
	return nil, s.err
}

type passwordChangeFixture struct {
	service    *Service
	credential *passwordChangeStore
	registry   *passwordChangeRegistryStore
	factor     *authTestTwoFactorStore
	events     []string
}

func newPasswordChangeFixture(t *testing.T) *passwordChangeFixture {
	t.Helper()
	hash, err := HashPassword(changeTestCurrent)
	if err != nil {
		t.Fatal(err)
	}
	base := &authTestStore{local: &LocalAdminCredential{Email: changeTestEmail, PasswordHash: hash}}
	f := &passwordChangeFixture{}
	f.service = newAuthTestService(t, base, newAuthTestUsers(), User{})
	f.credential = &passwordChangeStore{authTestStore: base, events: &f.events}
	f.service.local.store = f.credential
	f.registry = &passwordChangeRegistryStore{authTestSessionRegistryStore: newAuthTestSessionRegistryStore(), events: &f.events, failure: errors.New(changeTestCurrent + " " + changeTestNew)}
	f.registry.records[changeTestEmail] = SessionRegistryRecord{Preferences: SecurityPreferences{HistoryEnabled: true}}
	f.service.registry = newSessionRegistry(f.registry, 20)
	f.factor = newAuthTestTwoFactorStore()
	f.service.twoFactor = newTwoFactorAuthenticator(f.factor, "test", []byte("key"), time.Minute, 10)
	return f
}
func (f *passwordChangeFixture) enableTwoFactor(t *testing.T) string {
	t.Helper()
	codes, hashes, err := newRecoveryCodeSet(2)
	if err != nil {
		t.Fatal(err)
	}
	f.factor.records[changeTestEmail] = TwoFactorRecord{Secret: []byte("12345678901234567890"), RecoveryCodeHashes: hashes}
	return codes[0]
}

func TestChangePasswordFailureContractAndOrder(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setup   func(*testing.T, *passwordChangeFixture, *PasswordChange, *User)
		want    error
		events  []string
		changed bool
	}{
		{name: "google only", setup: func(_ *testing.T, _ *passwordChangeFixture, _ *PasswordChange, u *User) {
			u.Email = "google@example.com"
		}, want: ErrPasswordSignInUnavailable},
		{name: "wrong current", setup: func(_ *testing.T, _ *passwordChangeFixture, c *PasswordChange, _ *User) { c.Current = "wrong password" }, want: ErrCurrentPasswordIncorrect},
		{name: "2FA required", setup: func(t *testing.T, f *passwordChangeFixture, _ *PasswordChange, _ *User) { f.enableTwoFactor(t) }, want: ErrTwoFactorCodeRequired},
		{name: "2FA wrong", setup: func(t *testing.T, f *passwordChangeFixture, c *PasswordChange, _ *User) {
			f.enableTwoFactor(t)
			c.Code = "invalid"
		}, want: ErrInvalidTwoFactorCode},
		{name: "2FA replayed", setup: func(t *testing.T, f *passwordChangeFixture, c *PasswordChange, _ *User) {
			f.enableTwoFactor(t)
			r := f.factor.records[changeTestEmail]
			r.LastUsedTOTPCounter = uint64(time.Now().Unix()/30 + 1)
			f.factor.records[changeTestEmail] = r
			c.Code = TOTPCode(r.Secret, time.Now())
		}, want: ErrTwoFactorCodeReused},
		{name: "2FA read failed", setup: func(_ *testing.T, f *passwordChangeFixture, _ *PasswordChange, _ *User) {
			f.service.twoFactor.store = failingPasswordChangeTwoFactorStore{f.factor, errors.New(changeTestCurrent + changeTestNew)}
		}, want: ErrPasswordNotChanged},
		{name: "2FA save failed", setup: func(t *testing.T, f *passwordChangeFixture, c *PasswordChange, _ *User) {
			c.Code = f.enableTwoFactor(t)
			f.factor.saveErr = errors.New(changeTestCurrent + changeTestNew)
		}, want: ErrPasswordNotChanged},
		{name: "reuse", setup: func(_ *testing.T, _ *passwordChangeFixture, c *PasswordChange, _ *User) { c.New = c.Current }, want: ErrPasswordReuse},
		{name: "short", setup: func(_ *testing.T, _ *passwordChangeFixture, c *PasswordChange, _ *User) { c.New = "short" }, want: ErrPasswordTooShort},
		{name: "long", setup: func(_ *testing.T, _ *passwordChangeFixture, c *PasswordChange, _ *User) {
			c.New = strings.Repeat("x", 1025)
		}, want: ErrPasswordTooLong},
		{name: "revoke save failed", setup: func(_ *testing.T, f *passwordChangeFixture, _ *PasswordChange, _ *User) { f.registry.failCall = 1 }, want: ErrPasswordNotChanged, events: []string{"revoke"}},
		{name: "revoke load failed", setup: func(_ *testing.T, f *passwordChangeFixture, _ *PasswordChange, _ *User) {
			f.service.registry.store = failingSessionRegistryReadStore{f.registry.authTestSessionRegistryStore}
		}, want: ErrPasswordNotChanged},
		{name: "credential save failed", setup: func(_ *testing.T, f *passwordChangeFixture, _ *PasswordChange, _ *User) {
			f.credential.replaceErr = errors.New(changeTestCurrent + changeTestNew)
		}, want: ErrPasswordNotChangedSessionsRevoked, events: []string{"revoke", "save"}},
		{name: "credential changed", setup: func(_ *testing.T, f *passwordChangeFixture, _ *PasswordChange, _ *User) {
			f.credential.replaceErr = ErrLocalAdminCredentialChanged
		}, want: ErrLocalAdminCredentialChanged, events: []string{"revoke", "save"}},
		{name: "session reissue failed", setup: func(_ *testing.T, f *passwordChangeFixture, _ *PasswordChange, _ *User) { f.registry.failCall = 2 }, want: ErrPasswordChangedSessionFailed, events: []string{"revoke", "save", "reissue"}, changed: true},
		{name: "success", events: []string{"revoke", "save", "reissue"}, changed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPasswordChangeFixture(t)
			change := PasswordChange{Current: changeTestCurrent, New: changeTestNew}
			user := localAdminUser(changeTestEmail)
			if tc.setup != nil {
				tc.setup(t, f, &change, &user)
			}
			var logs bytes.Buffer
			oldLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			defer slog.SetDefault(oldLogger)
			cookie, err := f.service.ChangePassword(context.Background(), user, change, "1.2.3.4", "browser")
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if !reflect.DeepEqual(f.events, tc.events) {
				t.Fatalf("events = %v, want %v", f.events, tc.events)
			}
			if (cookie != "") != (tc.want == nil) {
				t.Fatalf("cookie presence inconsistent with result: error=%v", err)
			}
			for _, secret := range []string{changeTestCurrent, changeTestNew} {
				if strings.Contains(logs.String(), secret) || (err != nil && strings.Contains(err.Error(), secret)) {
					t.Fatal("password leaked to log or error")
				}
				if strings.Contains(f.credential.local.PasswordHash, secret) {
					t.Fatal("plaintext password persisted")
				}
			}
			password := changeTestCurrent
			if tc.changed {
				password = changeTestNew
			}
			ok, verifyErr := VerifyPassword(f.credential.local.PasswordHash, password)
			if verifyErr != nil || !ok {
				t.Fatalf("durable password inconsistent with result: %v", verifyErr)
			}
			if _, loginErr := f.service.LoginLocal(context.Background(), changeTestEmail, password); loginErr != nil {
				t.Fatalf("cached password inconsistent with result: %v", loginErr)
			}
		})
	}
}

func TestChangePasswordLoginRevocationAndSummary(t *testing.T) {
	f := newPasswordChangeFixture(t)
	old := f.service.codec.payload.sign(Session{Email: changeTestEmail, Sub: "local-admin", Iat: time.Now().Add(-time.Minute).Unix(), Exp: time.Now().Add(time.Hour).Unix()})
	cookie, err := f.service.ChangePassword(context.Background(), localAdminUser(changeTestEmail), PasswordChange{Current: changeTestCurrent, New: changeTestNew}, "1.2.3.4", "browser")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.CurrentSession(context.Background(), old); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("old session = %v", err)
	}
	if _, err := f.service.CurrentSession(context.Background(), cookie); err != nil {
		t.Fatalf("fresh session = %v", err)
	}
	if _, err := f.service.CompletePasswordLogin(context.Background(), changeTestEmail, changeTestCurrent, "", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old login = %v", err)
	}
	login, err := f.service.CompletePasswordLogin(context.Background(), changeTestEmail, changeTestNew, "", "")
	if err != nil || !login.Completed || login.CookieValue == "" {
		t.Fatalf("new login = %+v, %v", login, err)
	}
	history, err := f.service.registry.History(context.Background(), changeTestEmail)
	if err != nil || len(history.Entries) != 2 || history.Entries[1].Method != SignInMethodPassword || history.Entries[1].IP != "1.2.3.4" || history.Entries[1].UserAgent != "browser" {
		t.Fatalf("history = %+v, %v", history, err)
	}
	for _, tc := range []struct {
		email string
		want  bool
	}{{changeTestEmail, true}, {"google@example.com", false}} {
		summary, err := f.service.SecuritySummary(context.Background(), tc.email)
		if err != nil || summary.PasswordSignIn != tc.want {
			t.Fatalf("summary for %s = %+v, %v", tc.email, summary, err)
		}
	}
}

func TestChangePasswordConsumesRecoveryCode(t *testing.T) {
	f := newPasswordChangeFixture(t)
	code := f.enableTwoFactor(t)
	if _, err := f.service.ChangePassword(context.Background(), localAdminUser(changeTestEmail), PasswordChange{Current: changeTestCurrent, New: changeTestNew, Code: code}, "", ""); err != nil {
		t.Fatal(err)
	}
	if got := f.service.twoFactor.RecoveryCodesRemaining(context.Background(), changeTestEmail); got != 1 {
		t.Fatalf("remaining codes = %d, want 1", got)
	}
	if _, err := f.service.twoFactor.VerifyChallenge(context.Background(), changeTestEmail, code); !errors.Is(err, ErrInvalidTwoFactorCode) {
		t.Fatalf("reused recovery code = %v", err)
	}
}

func TestConcurrentChangePasswordHasOneWinner(t *testing.T) {
	f := newPasswordChangeFixture(t)
	// Single-session tracking makes any accidental loser reissue/revoke visible.
	f.registry.records[changeTestEmail] = SessionRegistryRecord{Preferences: SecurityPreferences{SingleSessionEnabled: true}}
	start := make(chan struct{})
	type result struct {
		cookie string
		err    error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			cookie, err := f.service.ChangePassword(context.Background(), localAdminUser(changeTestEmail), PasswordChange{Current: changeTestCurrent, New: changeTestNew}, "", "")
			results <- result{cookie, err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	wins, losses := 0, 0
	for r := range results {
		if r.err == nil {
			wins++
			if _, err := f.service.CurrentSession(context.Background(), r.cookie); err != nil {
				t.Fatalf("winning cookie invalid: %v", err)
			}
		} else if errors.Is(r.err, ErrCurrentPasswordIncorrect) {
			losses++
		} else {
			t.Fatalf("unexpected error: %v", r.err)
		}
	}
	if wins != 1 || losses != 1 {
		t.Fatalf("wins=%d losses=%d", wins, losses)
	}
	if !reflect.DeepEqual(f.events, []string{"revoke", "save", "reissue"}) {
		t.Fatalf("loser modified state: %v", f.events)
	}
}

func TestChangePasswordAcceptsTOTPAndRejectsReplay(t *testing.T) {
	f := newPasswordChangeFixture(t)
	f.enableTwoFactor(t)
	record := f.factor.records[changeTestEmail]
	code := TOTPCode(record.Secret, time.Now())
	cookie, err := f.service.ChangePassword(context.Background(), localAdminUser(changeTestEmail), PasswordChange{Current: changeTestCurrent, New: changeTestNew, Code: code}, "", "")
	if err != nil || cookie == "" {
		t.Fatalf("TOTP change error = %v", err)
	}
	if _, err := f.service.twoFactor.VerifyChallenge(context.Background(), changeTestEmail, code); !errors.Is(err, ErrTwoFactorCodeReused) {
		t.Fatalf("TOTP replay = %v", err)
	}
	if got := f.service.twoFactor.RecoveryCodesRemaining(context.Background(), changeTestEmail); got != 2 {
		t.Fatalf("TOTP consumed recovery code: remaining %d", got)
	}
}

func TestChangePasswordVerifiesPasswordBeforeTwoFactor(t *testing.T) {
	f := newPasswordChangeFixture(t)
	f.service.twoFactor.store = failingPasswordChangeTwoFactorStore{f.factor, errors.New("must not load 2FA")}
	_, err := f.service.ChangePassword(context.Background(), localAdminUser(changeTestEmail), PasswordChange{Current: "incorrect password", New: changeTestNew}, "", "")
	if !errors.Is(err, ErrCurrentPasswordIncorrect) {
		t.Fatalf("password did not take precedence: %v", err)
	}
	if len(f.events) != 0 {
		t.Fatalf("wrong password changed state: %v", f.events)
	}
}

// Pause a cold read after it captured absence, then complete enrollment before
// allowing the password-change workflow to decide whether proof is required.
type coldPasswordChangeFactorStore struct {
	TwoFactorStore
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (s *coldPasswordChangeFactorStore) Get(ctx context.Context, email string) (*TwoFactorRecord, error) {
	s.mu.Lock()
	s.calls++
	first := s.calls == 1
	record, err := s.TwoFactorStore.Get(ctx, email)
	s.mu.Unlock()
	if first {
		close(s.started)
		<-s.release
	}
	return record, err
}
func TestChangePasswordColdLoadCannotUndoEnrollment(t *testing.T) {
	f := newPasswordChangeFixture(t)
	store := &coldPasswordChangeFactorStore{TwoFactorStore: f.factor, started: make(chan struct{}), release: make(chan struct{})}
	f.service.twoFactor.store = store
	result := make(chan error, 1)
	go func() {
		_, err := f.service.ChangePassword(context.Background(), localAdminUser(changeTestEmail), PasswordChange{Current: changeTestCurrent, New: changeTestNew}, "", "")
		result <- err
	}()
	<-store.started
	enrollServiceAccount(t, f.service, changeTestEmail)
	close(store.release)
	if err := <-result; !errors.Is(err, ErrTwoFactorCodeRequired) {
		t.Fatalf("enrolled 2FA skipped: %v", err)
	}
	if !f.service.TwoFactorEnabled(context.Background(), changeTestEmail) {
		t.Fatal("cold read undid enrollment")
	}
	if len(f.events) != 0 {
		t.Fatalf("changed state without proof: %v", f.events)
	}
}
