package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

const testAccountProvider = agent.ProviderCodex

// testCredential is the opaque credential format the fakes understand: "id"
// is the stable account identity and "token" changes on every refresh.
func testCredential(id, token string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"id":%q,"token":%q}`, id, token))
}

func credentialID(credential json.RawMessage) string {
	var fields struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(credential, &fields)
	return fields.ID
}

func cloneCredential(credential json.RawMessage) json.RawMessage {
	if credential == nil {
		return nil
	}
	return append(json.RawMessage(nil), credential...)
}

// twoAccounts is a vault with the active account "work" (identity w) and an
// inactive account "home" (identity h).
func twoAccounts() AccountSet {
	return AccountSet{
		ActiveAccountID: "work",
		Accounts: []AccountRecord{
			{ID: "work", Label: "Work", Email: "w@example.com", Credential: testCredential("w", "t1")},
			{ID: "home", Label: "Home", Email: "h@example.com", Credential: testCredential("h", "t1")},
		},
	}
}

type fakeAccountStore struct {
	mu      sync.Mutex
	sets    map[agent.ProviderID]AccountSet
	loadErr error
	saveErr error
	loads   int
	saves   int
}

func newFakeAccountStore(set AccountSet) *fakeAccountStore {
	return &fakeAccountStore{sets: map[agent.ProviderID]AccountSet{testAccountProvider: set.Clone()}}
}

func (s *fakeAccountStore) AgentAccounts(_ context.Context, provider agent.ProviderID) (AccountSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loads++
	if s.loadErr != nil {
		return AccountSet{}, s.loadErr
	}
	return s.sets[provider].Clone(), nil
}

func (s *fakeAccountStore) SaveAgentAccounts(_ context.Context, provider agent.ProviderID, set AccountSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saves++
	s.sets[provider] = set.Clone()
	return nil
}

func (s *fakeAccountStore) failSaves(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveErr = err
}

// saved returns what the store last committed.
func (s *fakeAccountStore) saved() AccountSet {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sets[testAccountProvider].Clone()
}

func (s *fakeAccountStore) saveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saves
}

// fakeAccountCredentials keeps the host login in memory. Validation echoes
// the credential unless a test scripts it.
type fakeAccountCredentials struct {
	mu         sync.Mutex
	host       json.RawMessage
	readErr    error
	writeErr   error
	writes     int
	validated  []json.RawMessage
	validateFn func(json.RawMessage) (ValidatedAccount, error)
}

func (c *fakeAccountCredentials) ReadHost() (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readErr != nil {
		return nil, c.readErr
	}
	if c.host == nil {
		return nil, fmt.Errorf("open auth.json: %w", fs.ErrNotExist)
	}
	return cloneCredential(c.host), nil
}

func (c *fakeAccountCredentials) WriteHost(credential json.RawMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes++
	if c.writeErr != nil {
		return c.writeErr
	}
	c.host = cloneCredential(credential)
	return nil
}

func (c *fakeAccountCredentials) Validate(ctx context.Context, credential json.RawMessage) (ValidatedAccount, error) {
	c.mu.Lock()
	c.validated = append(c.validated, cloneCredential(credential))
	validate := c.validateFn
	c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ValidatedAccount{}, err
	}
	if validate != nil {
		return validate(credential)
	}
	id := credentialID(credential)
	return ValidatedAccount{Email: id + "@example.com", PlanType: "plus", Credential: cloneCredential(credential)}, nil
}

func (c *fakeAccountCredentials) Identity(credential json.RawMessage) AccountIdentity {
	return AccountIdentity{"id": credentialID(credential)}
}

func (c *fakeAccountCredentials) hostCredential() json.RawMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneCredential(c.host)
}

// setHost stands in for the CLI or a credential sync-back changing the host
// login.
func (c *fakeAccountCredentials) setHost(credential json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.host = cloneCredential(credential)
}

func (c *fakeAccountCredentials) failWrites(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeErr = err
}

func (c *fakeAccountCredentials) failReads(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readErr = err
}

func (c *fakeAccountCredentials) onValidate(validate func(json.RawMessage) (ValidatedAccount, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.validateFn = validate
}

func (c *fakeAccountCredentials) writeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writes
}

func (c *fakeAccountCredentials) validations() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	validated := make([]string, 0, len(c.validated))
	for _, credential := range c.validated {
		validated = append(validated, string(credential))
	}
	return validated
}

// refreshTo scripts a validation that refreshes the credential to token.
func refreshTo(token string) func(json.RawMessage) (ValidatedAccount, error) {
	return func(credential json.RawMessage) (ValidatedAccount, error) {
		id := credentialID(credential)
		return ValidatedAccount{Email: id + "@example.com", PlanType: "pro", Credential: testCredential(id, token)}, nil
	}
}

// signsInAs scripts a validation that reveals the credential belongs to id,
// whatever it claims.
func signsInAs(id string) func(json.RawMessage) (ValidatedAccount, error) {
	return func(json.RawMessage) (ValidatedAccount, error) {
		return ValidatedAccount{Email: id + "@example.com", Credential: testCredential(id, "real")}, nil
	}
}

func failValidation(err error) func(json.RawMessage) (ValidatedAccount, error) {
	return func(json.RawMessage) (ValidatedAccount, error) { return ValidatedAccount{}, err }
}

type fakeAccountLoginFlow struct {
	mu          sync.Mutex
	credentials *fakeAccountCredentials
	resetErr    error
	prepareErr  error
	startErr    error
	resets      int
	starts      int
	logins      []*fakeAccountLogin
	// undoHostWrites makes each login's Finish restore the host login it
	// found at Prepare, as a provider does for a CLI that ignored the
	// isolated location.
	undoHostWrites bool
}

func (f *fakeAccountLoginFlow) Reset(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resets++
	return f.resetErr
}

func (f *fakeAccountLoginFlow) Prepare() (AccountLogin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.prepareErr != nil {
		return nil, f.prepareErr
	}
	login := &fakeAccountLogin{marker: fmt.Sprintf("FAKE_LOGIN_HOME=/tmp/login-%d", len(f.logins)+1)}
	if f.undoHostWrites {
		login.credentials = f.credentials
		login.hostBefore = f.credentials.hostCredential()
	}
	f.logins = append(f.logins, login)
	return login, nil
}

func (f *fakeAccountLoginFlow) Start(context.Context) (LoginSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	if f.startErr != nil {
		return LoginSnapshot{}, f.startErr
	}
	return LoginSnapshot{Active: true, URL: "https://login.example/account"}, nil
}

func (f *fakeAccountLoginFlow) set(update func(*fakeAccountLoginFlow)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	update(f)
}

func (f *fakeAccountLoginFlow) counts() (resets, prepares, starts int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resets, len(f.logins), f.starts
}

func (f *fakeAccountLoginFlow) login(t *testing.T, index int) *fakeAccountLogin {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if index >= len(f.logins) {
		t.Fatalf("login %d was not prepared; %d were", index, len(f.logins))
	}
	return f.logins[index]
}

type fakeAccountLogin struct {
	mu          sync.Mutex
	marker      string
	credential  json.RawMessage
	err         error
	credentials *fakeAccountCredentials
	hostBefore  json.RawMessage
	finishes    int
	aborts      int
	exitErr     error
	output      string
}

func (l *fakeAccountLogin) Env(base []string) []string {
	return append(append([]string(nil), base...), l.marker)
}

func (l *fakeAccountLogin) Finish(exitErr error, output string) (json.RawMessage, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.finishes++
	l.exitErr, l.output = exitErr, output
	if l.credentials != nil && !bytes.Equal(l.credentials.hostCredential(), l.hostBefore) {
		l.credentials.setHost(l.hostBefore)
	}
	if l.err != nil {
		return nil, l.err
	}
	if exitErr != nil {
		return nil, fmt.Errorf("login failed: %w", exitErr)
	}
	if l.credential == nil {
		return nil, errors.New("login wrote no credential")
	}
	return cloneCredential(l.credential), nil
}

func (l *fakeAccountLogin) Abort() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.aborts++
}

// finishWith scripts what Finish collects from the isolated location.
func (l *fakeAccountLogin) finishWith(credential json.RawMessage, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.credential, l.err = cloneCredential(credential), err
}

func (l *fakeAccountLogin) calls() (finishes, aborts int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.finishes, l.aborts
}

type accountHarness struct {
	store       *fakeAccountStore
	credentials *fakeAccountCredentials
	flow        *fakeAccountLoginFlow
	service     *AccountService
	changes     atomic.Int32
	// changedHook, when set before an operation, runs inside Changed.
	changedHook func()
}

// newAccountFakes builds the fakes without opening the service, so a test
// can inject failures that Open must survive.
func newAccountFakes(saved AccountSet, host json.RawMessage) *accountHarness {
	credentials := &fakeAccountCredentials{host: cloneCredential(host)}
	return &accountHarness{
		store:       newFakeAccountStore(saved),
		credentials: credentials,
		flow:        &fakeAccountLoginFlow{credentials: credentials},
	}
}

func (h *accountHarness) config() AccountConfig {
	return AccountConfig{
		Provider: testAccountProvider, Label: "Codex",
		Credentials: h.credentials, Login: h.flow,
		Changed: func() {
			h.changes.Add(1)
			if h.changedHook != nil {
				h.changedHook()
			}
		},
	}
}

func (h *accountHarness) open(t *testing.T, configure ...func(*AccountConfig)) *accountHarness {
	t.Helper()
	config := h.config()
	for _, apply := range configure {
		apply(&config)
	}
	service, err := NewAccountVault(h.store).Open(context.Background(), config)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	h.service = service
	return h
}

func newAccountHarness(t *testing.T, saved AccountSet, host json.RawMessage, configure ...func(*AccountConfig)) *accountHarness {
	t.Helper()
	return newAccountFakes(saved, host).open(t, configure...)
}

func loginMayWriteHost(config *AccountConfig) { config.LoginMayWriteHost = true }

func replacePendingLogin(config *AccountConfig) { config.ReplacePendingLogin = true }

func (h *accountHarness) requireHost(t *testing.T, want json.RawMessage) {
	t.Helper()
	if got := h.credentials.hostCredential(); !bytes.Equal(got, want) {
		t.Fatalf("host credential = %s, want %s", got, want)
	}
}

// requireActive checks that memory and the store agree on the active account.
func (h *accountHarness) requireActive(t *testing.T, id string) {
	t.Helper()
	if got := h.service.AccountsSnapshot().ActiveAccountID; got != id {
		t.Fatalf("snapshot active account = %q, want %q", got, id)
	}
	if got := h.store.saved().ActiveAccountID; got != id {
		t.Fatalf("stored active account = %q, want %q", got, id)
	}
}

func (h *accountHarness) requireSavedCredential(t *testing.T, id string, want json.RawMessage) {
	t.Helper()
	record, ok := h.store.saved().Find(id)
	if !ok {
		t.Fatalf("stored accounts lack %q", id)
	}
	if !bytes.Equal(record.Credential, want) {
		t.Fatalf("stored credential of %q = %s, want %s", id, record.Credential, want)
	}
}

func (h *accountHarness) requireNoSaves(t *testing.T) {
	t.Helper()
	if saves := h.store.saveCount(); saves != 0 {
		t.Fatalf("store saved %d times, want none", saves)
	}
}

func (h *accountHarness) requireNoPendingLogin(t *testing.T) {
	t.Helper()
	if env, ok := h.service.LoginEnv([]string{"BASE=1"}); ok {
		t.Fatalf("LoginEnv = %v while no account login should be pending", env)
	}
}

func (h *accountHarness) beginRun(t *testing.T) func() {
	t.Helper()
	release, err := h.service.BeginRun()
	if err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	return release
}

func (h *accountHarness) startLogin(t *testing.T, label, accountID string) *fakeAccountLogin {
	t.Helper()
	if _, err := h.service.StartAccountLogin(context.Background(), label, accountID); err != nil {
		t.Fatalf("StartAccountLogin(%q, %q): %v", label, accountID, err)
	}
	_, prepares, _ := h.flow.counts()
	return h.flow.login(t, prepares-1)
}

func snapshotItem(t *testing.T, snapshot AccountsSnapshot, label string) Account {
	t.Helper()
	for _, item := range snapshot.Items {
		if item.Label == label {
			return item
		}
	}
	t.Fatalf("snapshot %#v lacks account %q", snapshot, label)
	return Account{}
}

func TestAccountIdentityKnown(t *testing.T) {
	for _, test := range []struct {
		name     string
		identity AccountIdentity
		want     bool
	}{
		{"nil", nil, false},
		{"empty", AccountIdentity{}, false},
		{"only empty values", AccountIdentity{"account": "", "email": ""}, false},
		{"one identifier", AccountIdentity{"account": "a"}, true},
		{"one of several", AccountIdentity{"account": "", "email": "a@example.com"}, true},
	} {
		if got := test.identity.Known(); got != test.want {
			t.Errorf("%s: Known() = %t, want %t", test.name, got, test.want)
		}
	}
}

func TestAccountIdentitySame(t *testing.T) {
	for _, test := range []struct {
		name string
		a, b AccountIdentity
		want bool
	}{
		{"same identifier", AccountIdentity{"account": "a"}, AccountIdentity{"account": "a"}, true},
		{"different identifier", AccountIdentity{"account": "a"}, AccountIdentity{"account": "b"}, false},
		{"shared key agrees, extra key on one side", AccountIdentity{"account": "a", "email": "x@example.com"}, AccountIdentity{"account": "a"}, true},
		{"one shared key disagrees", AccountIdentity{"account": "a", "email": "x@example.com"}, AccountIdentity{"account": "a", "email": "y@example.com"}, false},
		{"no shared key", AccountIdentity{"account": "a"}, AccountIdentity{"email": "x@example.com"}, false},
		{"empty value on one side", AccountIdentity{"account": "a", "email": ""}, AccountIdentity{"account": "a", "email": "x@example.com"}, true},
		{"only empty values", AccountIdentity{"account": ""}, AccountIdentity{"account": ""}, false},
		{"known against empty value", AccountIdentity{"account": "a"}, AccountIdentity{"account": ""}, false},
		{"nil identities", nil, nil, false},
		{"known against nil", AccountIdentity{"account": "a"}, nil, false},
	} {
		if got := test.a.Same(test.b); got != test.want {
			t.Errorf("%s: a.Same(b) = %t, want %t", test.name, got, test.want)
		}
		if got := test.b.Same(test.a); got != test.want {
			t.Errorf("%s: b.Same(a) = %t, want %t", test.name, got, test.want)
		}
	}
}

func TestAccountServiceImportCurrentRequiresHostLogin(t *testing.T) {
	h := newAccountHarness(t, AccountSet{}, nil)

	err := h.service.ImportCurrent(context.Background(), "Work")
	if err == nil || err.Error() != "Codex is not signed in" {
		t.Fatalf("ImportCurrent while signed out = %v", err)
	}

	unreadable := errors.New("permission denied")
	h.credentials.failReads(unreadable)
	err = h.service.ImportCurrent(context.Background(), "Work")
	if !errors.Is(err, unreadable) || !strings.Contains(err.Error(), "read current Codex credential") {
		t.Fatalf("ImportCurrent with an unreadable host = %v", err)
	}
	if validated := h.credentials.validations(); len(validated) != 0 {
		t.Fatalf("validated %v without a host login", validated)
	}
	h.requireNoSaves(t)
}

func TestAccountServiceImportCurrentRejectsLabelsBeforeValidating(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("n", "t1"))

	for _, test := range []struct {
		label string
		want  error
	}{
		{"", ErrAccountLabelRequired},
		{"   ", ErrAccountLabelRequired},
		{strings.Repeat("é", 65), ErrAccountLabelInvalid},
		{" WORK ", ErrAccountLabelConflict},
		{"home", ErrAccountLabelConflict},
	} {
		if err := h.service.ImportCurrent(context.Background(), test.label); !errors.Is(err, test.want) {
			t.Errorf("ImportCurrent(%q) = %v, want %v", test.label, err, test.want)
		}
	}
	if validated := h.credentials.validations(); len(validated) != 0 {
		t.Fatalf("validated %v for rejected labels", validated)
	}
	h.requireNoSaves(t)
	h.requireHost(t, testCredential("n", "t1"))
}

func TestAccountServiceImportCurrentRefusedDuringRun(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	release := h.beginRun(t)
	defer release()
	h.credentials.setHost(testCredential("n", "t1"))

	if err := h.service.ImportCurrent(context.Background(), "New"); !errors.Is(err, ErrAccountInUse) {
		t.Fatalf("ImportCurrent during a run = %v, want ErrAccountInUse", err)
	}
	if validated := h.credentials.validations(); len(validated) != 0 {
		t.Fatalf("validated %v during a run", validated)
	}
	h.requireNoSaves(t)
	h.requireActive(t, "work")
}

func TestAccountServiceImportCurrentSavesValidatedLoginAsActive(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("n", "t1"))
	h.credentials.onValidate(refreshTo("t2"))

	if err := h.service.ImportCurrent(context.Background(), "  New  "); err != nil {
		t.Fatalf("ImportCurrent: %v", err)
	}

	snapshot := h.service.AccountsSnapshot()
	if len(snapshot.Items) != 3 {
		t.Fatalf("snapshot = %#v, want three accounts", snapshot)
	}
	item := snapshotItem(t, snapshot, "New")
	if !item.Active || snapshot.ActiveAccountID != item.ID || item.ID == "" ||
		item.Email != "n@example.com" || item.PlanType != "pro" || item.ValidatedAt == nil {
		t.Fatalf("imported account = %#v in %#v", item, snapshot)
	}
	h.requireActive(t, item.ID)
	h.requireSavedCredential(t, item.ID, testCredential("n", "t2"))
	h.requireHost(t, testCredential("n", "t2"))
	if validated := h.credentials.validations(); len(validated) != 1 || validated[0] != string(testCredential("n", "t1")) {
		t.Fatalf("validated %v, want the host login once", validated)
	}
	if changes := h.changes.Load(); changes != 1 {
		t.Fatalf("Changed called %d times, want 1", changes)
	}
}

func TestAccountServiceImportCurrentRequiresIdentity(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("n", "t1"))
	h.credentials.onValidate(func(json.RawMessage) (ValidatedAccount, error) {
		return ValidatedAccount{Email: "n@example.com", Credential: json.RawMessage(`{"token":"t2"}`)}, nil
	})

	err := h.service.ImportCurrent(context.Background(), "New")
	if err == nil || !strings.Contains(err.Error(), "could not tell which Codex account") {
		t.Fatalf("ImportCurrent of an identity-less login = %v", err)
	}
	h.requireNoSaves(t)
	h.requireHost(t, testCredential("n", "t1"))
	if items := h.service.AccountsSnapshot().Items; len(items) != 2 {
		t.Fatalf("accounts = %#v, want the original two", items)
	}
}

func TestAccountServiceImportCurrentStoreFailureChangesNothing(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("n", "t1"))
	h.credentials.onValidate(refreshTo("t2"))
	saveErr := errors.New("disk full")
	h.store.failSaves(saveErr)
	writes := h.credentials.writeCount()

	if err := h.service.ImportCurrent(context.Background(), "New"); !errors.Is(err, saveErr) {
		t.Fatalf("ImportCurrent with a failing store = %v", err)
	}
	snapshot := h.service.AccountsSnapshot()
	if len(snapshot.Items) != 2 || snapshot.ActiveAccountID != "work" {
		t.Fatalf("snapshot after a failed save = %#v", snapshot)
	}
	h.requireHost(t, testCredential("n", "t1"))
	if h.credentials.writeCount() != writes {
		t.Fatal("the host was written although the store refused the account")
	}
}

func TestAccountServiceActivateAccountRejectsUnknownAndRunning(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))

	if err := h.service.ActivateAccount(context.Background(), "missing"); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("ActivateAccount(missing) = %v, want ErrAccountNotFound", err)
	}
	release := h.beginRun(t)
	if err := h.service.ActivateAccount(context.Background(), "home"); !errors.Is(err, ErrAccountInUse) {
		t.Fatalf("ActivateAccount during a run = %v, want ErrAccountInUse", err)
	}
	release()

	if validated := h.credentials.validations(); len(validated) != 0 {
		t.Fatalf("validated %v for refused activations", validated)
	}
	h.requireNoSaves(t)
	h.requireActive(t, "work")
	h.requireHost(t, testCredential("w", "t1"))
}

func TestAccountServiceActivateAccountValidatesBeforeTouchingHost(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	var hostDuringValidation json.RawMessage
	h.credentials.onValidate(func(credential json.RawMessage) (ValidatedAccount, error) {
		hostDuringValidation = h.credentials.hostCredential()
		return refreshTo("t2")(credential)
	})

	if err := h.service.ActivateAccount(context.Background(), "home"); err != nil {
		t.Fatalf("ActivateAccount: %v", err)
	}
	if !bytes.Equal(hostDuringValidation, testCredential("w", "t1")) {
		t.Fatalf("host during validation = %s, want the outgoing login", hostDuringValidation)
	}
	h.requireActive(t, "home")
	h.requireSavedCredential(t, "home", testCredential("h", "t2"))
	h.requireHost(t, testCredential("h", "t2"))
	item := snapshotItem(t, h.service.AccountsSnapshot(), "Home")
	if !item.Active || item.PlanType != "pro" || item.ValidatedAt == nil {
		t.Fatalf("activated account = %#v", item)
	}
	if changes := h.changes.Load(); changes != 1 {
		t.Fatalf("Changed called %d times, want 1", changes)
	}
}

func TestAccountServiceActivateAccountValidationFailureChangesNothing(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	rejected := errors.New("token revoked")
	h.credentials.onValidate(failValidation(rejected))

	if err := h.service.ActivateAccount(context.Background(), "home"); !errors.Is(err, rejected) {
		t.Fatalf("ActivateAccount with a rejected login = %v", err)
	}
	h.requireNoSaves(t)
	h.requireActive(t, "work")
	h.requireHost(t, testCredential("w", "t1"))
}

func TestAccountServiceActivateAccountRejectsIdentityChange(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.onValidate(signsInAs("x"))

	err := h.service.ActivateAccount(context.Background(), "home")
	if !errors.Is(err, ErrAccountIdentityMismatch) {
		t.Fatalf("ActivateAccount of a login for another account = %v, want ErrAccountIdentityMismatch", err)
	}
	h.requireNoSaves(t)
	h.requireActive(t, "work")
	h.requireSavedCredential(t, "home", testCredential("h", "t1"))
	h.requireHost(t, testCredential("w", "t1"))
}

func TestAccountServiceActivateAccountKeepsOutgoingRefreshedLogin(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	// The CLI refreshed the outgoing account's login on the host.
	h.credentials.setHost(testCredential("w", "t5"))

	if err := h.service.ActivateAccount(context.Background(), "home"); err != nil {
		t.Fatalf("ActivateAccount: %v", err)
	}
	h.requireSavedCredential(t, "work", testCredential("w", "t5"))
	h.requireActive(t, "home")
	h.requireHost(t, testCredential("h", "t1"))
	want := []string{string(testCredential("w", "t5")), string(testCredential("h", "t1"))}
	if validated := h.credentials.validations(); strings.Join(validated, " ") != strings.Join(want, " ") {
		t.Fatalf("validated %v, want %v", validated, want)
	}
}

func TestAccountServiceActivateAccountDoesNotCaptureForeignHostLogin(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("x", "t1"))

	if err := h.service.ActivateAccount(context.Background(), "home"); err != nil {
		t.Fatalf("ActivateAccount: %v", err)
	}
	saved := h.store.saved()
	for _, record := range saved.Accounts {
		if credentialID(record.Credential) == "x" {
			t.Fatalf("foreign host login entered the vault as %q", record.ID)
		}
	}
	h.requireSavedCredential(t, "work", testCredential("w", "t1"))
	h.requireActive(t, "home")
	h.requireHost(t, testCredential("h", "t1"))
	if validated := h.credentials.validations(); len(validated) != 1 || validated[0] != string(testCredential("h", "t1")) {
		t.Fatalf("validated %v, want only the target account", validated)
	}
}

func TestAccountServiceActivateAccountProceedsWhenOutgoingLoginIsRejected(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("w", "t5"))
	h.credentials.onValidate(func(credential json.RawMessage) (ValidatedAccount, error) {
		if credentialID(credential) == "w" {
			return ValidatedAccount{}, errors.New("temporarily unavailable")
		}
		return refreshTo("t2")(credential)
	})

	if err := h.service.ActivateAccount(context.Background(), "home"); err != nil {
		t.Fatalf("ActivateAccount: %v", err)
	}
	h.requireSavedCredential(t, "work", testCredential("w", "t1"))
	h.requireActive(t, "home")
	h.requireHost(t, testCredential("h", "t2"))
}

func TestAccountServiceActivateAccountHostWriteFailureIsRetriedBeforeRuns(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.failWrites(errors.New("read-only file system"))

	err := h.service.ActivateAccount(context.Background(), "home")
	if !errors.Is(err, ErrAccountNotApplied) {
		t.Fatalf("ActivateAccount with a failing host write = %v, want ErrAccountNotApplied", err)
	}
	// The store is the authority: the selection is committed even though
	// the host still holds the old login.
	h.requireActive(t, "home")
	h.requireHost(t, testCredential("w", "t1"))

	writes := h.credentials.writeCount()
	release, err := h.service.BeginRun()
	if !errors.Is(err, ErrAccountNotApplied) || release != nil {
		t.Fatalf("BeginRun with a stale host = (%v, %v), want a refusal", release != nil, err)
	}
	if h.credentials.writeCount() != writes+1 {
		t.Fatal("BeginRun did not retry the host write")
	}
	// A refused run holds no lease, so the account can still be changed.
	if err := h.service.ActivateAccount(context.Background(), "home"); !errors.Is(err, ErrAccountNotApplied) {
		t.Fatalf("ActivateAccount after a refused run = %v, want ErrAccountNotApplied", err)
	}

	h.credentials.failWrites(nil)
	release = h.beginRun(t)
	release()
	h.requireHost(t, testCredential("h", "t1"))
	writes = h.credentials.writeCount()
	release = h.beginRun(t)
	release()
	if h.credentials.writeCount() != writes {
		t.Fatal("BeginRun rewrote a host login that was already applied")
	}
}

func TestAccountServiceDeleteAccount(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))

	if err := h.service.DeleteAccount(context.Background(), "work"); !errors.Is(err, ErrActiveAccountDelete) {
		t.Fatalf("DeleteAccount(active) = %v, want ErrActiveAccountDelete", err)
	}
	if err := h.service.DeleteAccount(context.Background(), "missing"); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("DeleteAccount(missing) = %v, want ErrAccountNotFound", err)
	}
	h.requireNoSaves(t)

	saveErr := errors.New("disk full")
	h.store.failSaves(saveErr)
	if err := h.service.DeleteAccount(context.Background(), "home"); !errors.Is(err, saveErr) {
		t.Fatalf("DeleteAccount with a failing store = %v", err)
	}
	if items := h.service.AccountsSnapshot().Items; len(items) != 2 {
		t.Fatalf("accounts after a failed delete = %#v", items)
	}
	h.store.failSaves(nil)

	changes := h.changes.Load()
	writes := h.credentials.writeCount()
	if err := h.service.DeleteAccount(context.Background(), "home"); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, ok := h.store.saved().Find("home"); ok {
		t.Fatal("deleted account is still stored")
	}
	if items := h.service.AccountsSnapshot().Items; len(items) != 1 || items[0].ID != "work" {
		t.Fatalf("accounts after delete = %#v", items)
	}
	h.requireActive(t, "work")
	if h.changes.Load() != changes+1 {
		t.Fatal("DeleteAccount did not call Changed")
	}
	if h.credentials.writeCount() != writes {
		t.Fatal("deleting an inactive account wrote the host")
	}
}

func TestAccountServiceStartAccountLoginNormalizesLabels(t *testing.T) {
	for _, test := range []struct {
		name      string
		label     string
		accountID string
		wantErr   error
		wantLabel string
	}{
		{name: "new account label is trimmed", label: "  New  ", wantLabel: "New"},
		{name: "new account needs a label", label: "  ", wantErr: ErrAccountLabelRequired},
		{name: "label too long", label: strings.Repeat("x", 65), wantErr: ErrAccountLabelInvalid},
		{name: "new account label conflicts", label: "WORK", wantErr: ErrAccountLabelConflict},
		{name: "reconnect keeps saved label", label: " ", accountID: "home", wantLabel: "Home"},
		{name: "reconnect may restyle its own label", label: "HOME", accountID: "home", wantLabel: "HOME"},
		{name: "reconnect may rename", label: "Laptop", accountID: "home", wantLabel: "Laptop"},
		{name: "reconnect cannot take another label", label: "work", accountID: "home", wantErr: ErrAccountLabelConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
			_, err := h.service.StartAccountLogin(context.Background(), test.label, test.accountID)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("StartAccountLogin = %v, want %v", err, test.wantErr)
				}
				h.requireNoPendingLogin(t)
				if _, _, starts := h.flow.counts(); starts != 0 {
					t.Fatalf("provider login started %d times for a rejected label", starts)
				}
				return
			}
			if err != nil {
				t.Fatalf("StartAccountLogin: %v", err)
			}
			identity := "n"
			if test.accountID != "" {
				identity = "h"
			}
			h.flow.login(t, 0).finishWith(testCredential(identity, "t9"), nil)
			if handled, err := h.service.FinishLogin(nil, ""); !handled || err != nil {
				t.Fatalf("FinishLogin = (%t, %v)", handled, err)
			}
			item := snapshotItem(t, h.service.AccountsSnapshot(), test.wantLabel)
			if test.accountID != "" && item.ID != test.accountID {
				t.Fatalf("reconnected account got ID %q, want %q", item.ID, test.accountID)
			}
		})
	}
}

func TestAccountServiceStartAccountLoginRefusals(t *testing.T) {
	t.Run("unknown reconnect account", func(t *testing.T) {
		h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
		if _, err := h.service.StartAccountLogin(context.Background(), "", "missing"); !errors.Is(err, ErrAccountNotFound) {
			t.Fatalf("StartAccountLogin(missing) = %v, want ErrAccountNotFound", err)
		}
		h.requireNoPendingLogin(t)
		if _, prepares, starts := h.flow.counts(); prepares != 0 || starts != 0 {
			t.Fatalf("prepared %d and started %d logins", prepares, starts)
		}
	})
	t.Run("running", func(t *testing.T) {
		h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
		release := h.beginRun(t)
		defer release()
		if _, err := h.service.StartAccountLogin(context.Background(), "New", ""); !errors.Is(err, ErrAccountInUse) {
			t.Fatalf("StartAccountLogin during a run = %v, want ErrAccountInUse", err)
		}
		h.requireNoPendingLogin(t)
		if _, prepares, starts := h.flow.counts(); prepares != 0 || starts != 0 {
			t.Fatalf("prepared %d and started %d logins", prepares, starts)
		}
	})
	t.Run("reset failure", func(t *testing.T) {
		h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
		busy := errors.New("a Codex login is already in progress")
		h.flow.set(func(f *fakeAccountLoginFlow) { f.resetErr = busy })
		if _, err := h.service.StartAccountLogin(context.Background(), "New", ""); !errors.Is(err, busy) {
			t.Fatalf("StartAccountLogin with a failing reset = %v", err)
		}
		h.requireNoPendingLogin(t)
		if _, prepares, starts := h.flow.counts(); prepares != 0 || starts != 0 {
			t.Fatalf("prepared %d and started %d logins", prepares, starts)
		}
	})
	t.Run("prepare failure", func(t *testing.T) {
		h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
		noSpace := errors.New("no space left on device")
		h.flow.set(func(f *fakeAccountLoginFlow) { f.prepareErr = noSpace })
		_, err := h.service.StartAccountLogin(context.Background(), "New", "")
		if !errors.Is(err, noSpace) || !strings.Contains(err.Error(), "prepare Codex login") {
			t.Fatalf("StartAccountLogin with a failing prepare = %v", err)
		}
		h.requireNoPendingLogin(t)
	})
}

func TestAccountServiceLoginEnvOnlyWhilePending(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.requireNoPendingLogin(t)

	login := h.startLogin(t, "New", "")
	env, ok := h.service.LoginEnv([]string{"BASE=1"})
	if !ok || strings.Join(env, " ") != "BASE=1 "+login.marker {
		t.Fatalf("LoginEnv while pending = (%v, %t)", env, ok)
	}

	login.finishWith(nil, errors.New("cancelled"))
	if handled, _ := h.service.FinishLogin(nil, ""); !handled {
		t.Fatal("FinishLogin did not handle the pending login")
	}
	h.requireNoPendingLogin(t)
}

func TestAccountServiceStartFailureAbortsPendingLogin(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	startErr := errors.New("codex: command not found")
	h.flow.set(func(f *fakeAccountLoginFlow) { f.startErr = startErr })

	if _, err := h.service.StartAccountLogin(context.Background(), "New", ""); !errors.Is(err, startErr) {
		t.Fatalf("StartAccountLogin with a failing start = %v", err)
	}
	if _, aborts := h.flow.login(t, 0).calls(); aborts != 1 {
		t.Fatalf("failed login aborted %d times, want 1", aborts)
	}
	h.requireNoPendingLogin(t)

	h.flow.set(func(f *fakeAccountLoginFlow) { f.startErr = nil })
	h.startLogin(t, "New", "")
}

func TestAccountServiceSecondLoginWhilePending(t *testing.T) {
	t.Run("refused", func(t *testing.T) {
		h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
		first := h.startLogin(t, "First", "")

		_, err := h.service.StartAccountLogin(context.Background(), "Second", "")
		if !errors.Is(err, ErrAccountLoginInProgress) || err.Error() != "a Codex account login is already in progress" {
			t.Fatalf("second StartAccountLogin = %v, want ErrAccountLoginInProgress", err)
		}
		if _, aborts := first.calls(); aborts != 0 {
			t.Fatal("the refused login aborted the pending one")
		}
		if env, ok := h.service.LoginEnv(nil); !ok || env[0] != first.marker {
			t.Fatalf("LoginEnv = (%v, %t), want the first login", env, ok)
		}
	})
	t.Run("replaced", func(t *testing.T) {
		h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"), replacePendingLogin)
		first := h.startLogin(t, "First", "")
		second := h.startLogin(t, "Second", "")

		if _, aborts := first.calls(); aborts != 1 {
			t.Fatalf("replaced login aborted %d times, want 1", aborts)
		}
		if env, ok := h.service.LoginEnv(nil); !ok || env[0] != second.marker {
			t.Fatalf("LoginEnv = (%v, %t), want the second login", env, ok)
		}
		second.finishWith(testCredential("n", "t1"), nil)
		if handled, err := h.service.FinishLogin(nil, ""); !handled || err != nil {
			t.Fatalf("FinishLogin = (%t, %v)", handled, err)
		}
		if finishes, _ := first.calls(); finishes != 0 {
			t.Fatal("the replaced login was finished")
		}
		snapshotItem(t, h.service.AccountsSnapshot(), "Second")
	})
}

func TestAccountServiceFinishLoginWithoutPendingLogin(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	if handled, err := h.service.FinishLogin(nil, "plain login output"); handled || err != nil {
		t.Fatalf("FinishLogin without an account login = (%t, %v), want (false, nil)", handled, err)
	}
	h.requireNoSaves(t)
}

func TestAccountServiceFinishLoginFailureSavesNothing(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	login := h.startLogin(t, "New", "")
	exitErr := errors.New("exit status 1")

	handled, err := h.service.FinishLogin(exitErr, "login cancelled")
	if !handled || !errors.Is(err, exitErr) {
		t.Fatalf("FinishLogin of a failed login = (%t, %v)", handled, err)
	}
	login.mu.Lock()
	gotExit, gotOutput := login.exitErr, login.output
	login.mu.Unlock()
	if gotExit != exitErr || gotOutput != "login cancelled" {
		t.Fatalf("Finish received (%v, %q)", gotExit, gotOutput)
	}
	if validated := h.credentials.validations(); len(validated) != 0 {
		t.Fatalf("validated %v after a failed login", validated)
	}
	h.requireNoSaves(t)
	h.requireNoPendingLogin(t)
	h.requireHost(t, testCredential("w", "t1"))
}

func TestAccountServiceFinishLoginValidationFailureSavesNothing(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	login := h.startLogin(t, "New", "")
	login.finishWith(testCredential("n", "t1"), nil)
	h.credentials.onValidate(failValidation(errors.New("usage endpoint said: " + strings.Repeat("x", 1000))))

	handled, err := h.service.FinishLogin(nil, "")
	if !handled || err == nil {
		t.Fatalf("FinishLogin of a rejected login = (%t, %v)", handled, err)
	}
	if message := err.Error(); len(message) != accountErrorLimit+len("...") || !strings.HasSuffix(message, "...") ||
		!strings.HasPrefix(message, "usage endpoint said: ") {
		t.Fatalf("validation error was not shortened: %d bytes", len(message))
	}
	h.requireNoSaves(t)
	h.requireNoPendingLogin(t)
	if items := h.service.AccountsSnapshot().Items; len(items) != 2 {
		t.Fatalf("accounts = %#v, want the original two", items)
	}

	// Short errors keep their identity.
	login = h.startLogin(t, "New", "")
	login.finishWith(testCredential("n", "t1"), nil)
	rejected := errors.New("not a subscription account")
	h.credentials.onValidate(failValidation(rejected))
	if _, err := h.service.FinishLogin(nil, ""); !errors.Is(err, rejected) {
		t.Fatalf("FinishLogin = %v, want the validation error", err)
	}
}

func TestAccountServiceFinishLoginActivatesWhenIdle(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	login := h.startLogin(t, " New ", "")
	login.finishWith(testCredential("n", "t1"), nil)
	h.credentials.onValidate(refreshTo("t2"))
	changes := h.changes.Load()

	if handled, err := h.service.FinishLogin(nil, "Successfully logged in"); !handled || err != nil {
		t.Fatalf("FinishLogin = (%t, %v)", handled, err)
	}
	item := snapshotItem(t, h.service.AccountsSnapshot(), "New")
	if !item.Active || item.Email != "n@example.com" || item.ValidatedAt == nil {
		t.Fatalf("new account = %#v", item)
	}
	h.requireActive(t, item.ID)
	h.requireSavedCredential(t, item.ID, testCredential("n", "t2"))
	h.requireHost(t, testCredential("n", "t2"))
	h.requireNoPendingLogin(t)
	if h.changes.Load() != changes {
		t.Fatal("FinishLogin called Changed; the provider publishes the finished login")
	}
}

func TestAccountServiceFinishLoginDuringRunSavesWithoutActivating(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	login := h.startLogin(t, "New", "")
	release := h.beginRun(t)
	defer release()
	login.finishWith(testCredential("n", "t1"), nil)
	writes := h.credentials.writeCount()

	if handled, err := h.service.FinishLogin(nil, ""); !handled || err != nil {
		t.Fatalf("FinishLogin = (%t, %v)", handled, err)
	}
	item := snapshotItem(t, h.service.AccountsSnapshot(), "New")
	if item.Active {
		t.Fatal("a login finished during a run switched the active account")
	}
	h.requireSavedCredential(t, item.ID, testCredential("n", "t1"))
	h.requireActive(t, "work")
	h.requireHost(t, testCredential("w", "t1"))
	if h.credentials.writeCount() != writes {
		t.Fatal("a login finished during a run wrote the host")
	}
}

func TestAccountServiceReconnectReplacesSavedAccount(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	login := h.startLogin(t, "", "home")
	login.finishWith(testCredential("h", "t9"), nil)

	if handled, err := h.service.FinishLogin(nil, ""); !handled || err != nil {
		t.Fatalf("FinishLogin = (%t, %v)", handled, err)
	}
	snapshot := h.service.AccountsSnapshot()
	if len(snapshot.Items) != 2 {
		t.Fatalf("reconnect added an account: %#v", snapshot)
	}
	if item := snapshotItem(t, snapshot, "Home"); item.ID != "home" || !item.Active {
		t.Fatalf("reconnected account = %#v", item)
	}
	h.requireSavedCredential(t, "home", testCredential("h", "t9"))
	h.requireActive(t, "home")
	h.requireHost(t, testCredential("h", "t9"))
}

func TestAccountServiceReconnectOfDeletedAccount(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	login := h.startLogin(t, "", "home")
	if err := h.service.DeleteAccount(context.Background(), "home"); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	login.finishWith(testCredential("h", "t9"), nil)

	if handled, err := h.service.FinishLogin(nil, ""); !handled || !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("FinishLogin for a deleted account = (%t, %v), want ErrAccountNotFound", handled, err)
	}
	if saves := h.store.saveCount(); saves != 1 {
		t.Fatalf("store saved %d times, want only the delete", saves)
	}
	if _, ok := h.store.saved().Find("home"); ok {
		t.Fatal("the deleted account was saved again")
	}
	h.requireHost(t, testCredential("w", "t1"))
}

func TestAccountServiceLoginMayWriteHostBlocksRuns(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"), loginMayWriteHost)
	login := h.startLogin(t, "New", "")

	release, err := h.service.BeginRun()
	if !errors.Is(err, ErrAccountLoginInProgress) || release != nil {
		t.Fatalf("BeginRun during a blocking login = (%t, %v), want ErrAccountLoginInProgress", release != nil, err)
	}

	login.finishWith(nil, errors.New("cancelled"))
	if handled, _ := h.service.FinishLogin(nil, ""); !handled {
		t.Fatal("FinishLogin did not handle the pending login")
	}
	h.beginRun(t)()
}

func TestAccountServiceRunsProceedDuringNonBlockingLogin(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.startLogin(t, "New", "")
	h.beginRun(t)()
}

func TestAccountServiceRunLeases(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	ctx := context.Background()
	first := h.beginRun(t)
	second := h.beginRun(t)

	h.credentials.setHost(testCredential("n", "t1"))
	if err := h.service.ImportCurrent(ctx, "New"); !errors.Is(err, ErrAccountInUse) {
		t.Fatalf("ImportCurrent during runs = %v, want ErrAccountInUse", err)
	}
	h.credentials.setHost(testCredential("w", "t1"))
	if err := h.service.ActivateAccount(ctx, "home"); !errors.Is(err, ErrAccountInUse) {
		t.Fatalf("ActivateAccount during runs = %v, want ErrAccountInUse", err)
	}
	if _, err := h.service.StartAccountLogin(ctx, "New", ""); !errors.Is(err, ErrAccountInUse) {
		t.Fatalf("StartAccountLogin during runs = %v, want ErrAccountInUse", err)
	}

	// Releasing one lease twice must not release the other.
	first()
	first()
	if err := h.service.ActivateAccount(ctx, "home"); !errors.Is(err, ErrAccountInUse) {
		t.Fatalf("ActivateAccount with one run left = %v, want ErrAccountInUse", err)
	}
	second()
	if err := h.service.ActivateAccount(ctx, "home"); err != nil {
		t.Fatalf("ActivateAccount after all runs ended: %v", err)
	}
}

func TestAccountServiceBeginRunForSelectsTheRequestedAccountAtomically(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))

	release, err := h.service.BeginRunFor(context.Background(), "home")
	if err != nil {
		t.Fatalf("BeginRunFor: %v", err)
	}
	defer release()
	h.requireActive(t, "home")
	h.requireHost(t, testCredential("h", "t1"))
	if got := h.changes.Load(); got != 1 {
		t.Fatalf("account change notifications = %d, want 1", got)
	}

	if second, err := h.service.BeginRunFor(context.Background(), "work"); !errors.Is(err, ErrAccountInUse) || second != nil {
		t.Fatalf("BeginRunFor another account during the lease = (%t, %v), want ErrAccountInUse", second != nil, err)
	}
}

func TestAccountServiceBeginRunForRejectsAnUnknownAccount(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	if release, err := h.service.BeginRunFor(context.Background(), "missing"); !errors.Is(err, ErrAccountNotFound) || release != nil {
		t.Fatalf("BeginRunFor missing account = (%t, %v), want ErrAccountNotFound", release != nil, err)
	}
	h.requireActive(t, "work")
	h.requireHost(t, testCredential("w", "t1"))
}

func TestAccountServiceRunCredentialsDoNotSwitchTheSharedHost(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	legacyRelease := h.beginRun(t)
	defer legacyRelease()

	work, ok, err := h.service.CredentialForRun("work")
	if err != nil || !ok {
		t.Fatalf("CredentialForRun(work) = (%#v, %t, %v)", work, ok, err)
	}
	home, ok, err := h.service.CredentialForRun("home")
	if err != nil || !ok {
		t.Fatalf("CredentialForRun(home) during another run = (%#v, %t, %v)", home, ok, err)
	}
	if work.AccountID != "work" || home.AccountID != "home" || bytes.Equal(work.Credential, home.Credential) {
		t.Fatalf("run credentials = work %#v, home %#v", work, home)
	}
	h.requireActive(t, "work")
	h.requireHost(t, testCredential("w", "t1"))
	h.requireNoSaves(t)
}

func TestAccountServiceCapturesAnIsolatedRunIntoItsOwnAccount(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	run, ok, err := h.service.CredentialForRun("home")
	if err != nil || !ok {
		t.Fatalf("CredentialForRun(home) = (%#v, %t, %v)", run, ok, err)
	}
	if err := h.service.CaptureRunCredential(context.Background(), run, testCredential("h", "t2")); err != nil {
		t.Fatal(err)
	}
	h.requireSavedCredential(t, "home", testCredential("h", "t2"))
	h.requireSavedCredential(t, "work", testCredential("w", "t1"))
	h.requireActive(t, "work")
	h.requireHost(t, testCredential("w", "t1"))
}

func TestAccountServiceDeleteInactiveAccountDuringRun(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	release := h.beginRun(t)
	defer release()

	if err := h.service.DeleteAccount(context.Background(), "home"); err != nil {
		t.Fatalf("DeleteAccount of an inactive account during a run: %v", err)
	}
	h.requireHost(t, testCredential("w", "t1"))
}

func TestAccountServiceCaptureAfterRunWithoutActiveAccount(t *testing.T) {
	h := newAccountHarness(t, AccountSet{}, testCredential("n", "t1"))

	if err := h.service.CaptureAfterRun(context.Background()); err != nil {
		t.Fatalf("CaptureAfterRun: %v", err)
	}
	if validated := h.credentials.validations(); len(validated) != 0 {
		t.Fatalf("validated %v without an active account", validated)
	}
	h.requireNoSaves(t)
	h.requireHost(t, testCredential("n", "t1"))
}

func TestAccountServiceCaptureAfterRunUnchangedLogin(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))

	if err := h.service.CaptureAfterRun(context.Background()); err != nil {
		t.Fatalf("CaptureAfterRun: %v", err)
	}
	if validated := h.credentials.validations(); len(validated) != 0 {
		t.Fatalf("validated %v although the host login did not change", validated)
	}
	h.requireNoSaves(t)
}

func TestAccountServiceCaptureAfterRunSavesSameAccountLogin(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("w", "t2"))
	writes := h.credentials.writeCount()

	if err := h.service.CaptureAfterRun(context.Background()); err != nil {
		t.Fatalf("CaptureAfterRun: %v", err)
	}
	if validated := h.credentials.validations(); len(validated) != 1 || validated[0] != string(testCredential("w", "t2")) {
		t.Fatalf("validated %v, want the refreshed host login", validated)
	}
	h.requireSavedCredential(t, "work", testCredential("w", "t2"))
	h.requireActive(t, "work")
	h.requireHost(t, testCredential("w", "t2"))
	if h.credentials.writeCount() != writes {
		t.Fatal("the host was rewritten although validation did not refresh it")
	}
}

func TestAccountServiceCaptureAfterRunWritesLoginRefreshedByValidation(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("w", "t2"))
	h.credentials.onValidate(refreshTo("t3"))

	if err := h.service.CaptureAfterRun(context.Background()); err != nil {
		t.Fatalf("CaptureAfterRun: %v", err)
	}
	h.requireSavedCredential(t, "work", testCredential("w", "t3"))
	h.requireHost(t, testCredential("w", "t3"))
	if item := snapshotItem(t, h.service.AccountsSnapshot(), "Work"); item.PlanType != "pro" {
		t.Fatalf("captured account metadata = %#v", item)
	}
}

func TestAccountServiceCaptureAfterRunQuarantinesOtherAccountLogin(t *testing.T) {
	for _, test := range []struct {
		name string
		// host is what the run synchronized back; validate is what the
		// provider learns about it.
		host     json.RawMessage
		validate func(json.RawMessage) (ValidatedAccount, error)
	}{
		{name: "claims another account", host: testCredential("x", "t1")},
		// A container can forge the identity claims; the validated login
		// shows which account it really signs in as.
		{name: "forged claims", host: testCredential("w", "forged"), validate: signsInAs("x")},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
			h.credentials.setHost(test.host)
			h.credentials.onValidate(test.validate)

			err := h.service.CaptureAfterRun(context.Background())
			if !errors.Is(err, ErrAccountIdentityMismatch) {
				t.Fatalf("CaptureAfterRun = %v, want ErrAccountIdentityMismatch", err)
			}
			h.requireNoSaves(t)
			h.requireSavedCredential(t, "work", testCredential("w", "t1"))
			h.requireHost(t, testCredential("w", "t1"))
		})
	}
}

func TestAccountServiceCaptureAfterRunRestoreFailureIsRetriedBeforeRuns(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("x", "t1"))
	h.credentials.failWrites(errors.New("read-only file system"))

	err := h.service.CaptureAfterRun(context.Background())
	if !errors.Is(err, ErrAccountIdentityMismatch) || !errors.Is(err, ErrAccountNotApplied) {
		t.Fatalf("CaptureAfterRun = %v, want an identity mismatch that was not restored", err)
	}
	if _, err := h.service.BeginRun(); !errors.Is(err, ErrAccountNotApplied) {
		t.Fatalf("BeginRun with the foreign login still on the host = %v", err)
	}
	h.requireHost(t, testCredential("x", "t1"))

	h.credentials.failWrites(nil)
	h.beginRun(t)()
	h.requireHost(t, testCredential("w", "t1"))
	h.requireNoSaves(t)
}

func TestAccountServiceCaptureAfterRunLeavesRejectedLoginUnsaved(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("w", "t2"))
	unavailable := errors.New("provider unavailable")
	h.credentials.onValidate(failValidation(unavailable))
	writes := h.credentials.writeCount()

	if err := h.service.CaptureAfterRun(context.Background()); !errors.Is(err, unavailable) {
		t.Fatalf("CaptureAfterRun = %v, want the validation error", err)
	}
	h.requireNoSaves(t)
	h.requireHost(t, testCredential("w", "t2"))
	if h.credentials.writeCount() != writes {
		t.Fatal("a login that failed validation was replaced on the host")
	}
}

func TestAccountServiceCaptureAfterRunRequiresReadableHost(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	unreadable := errors.New("permission denied")
	h.credentials.failReads(unreadable)

	if err := h.service.CaptureAfterRun(context.Background()); !errors.Is(err, unreadable) {
		t.Fatalf("CaptureAfterRun with an unreadable host = %v", err)
	}
	h.requireNoSaves(t)
}

func TestAccountServiceCaptureAfterRunStoreFailureKeepsSavedLogin(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("w", "t2"))
	saveErr := errors.New("disk full")
	h.store.failSaves(saveErr)

	if err := h.service.CaptureAfterRun(context.Background()); !errors.Is(err, saveErr) {
		t.Fatalf("CaptureAfterRun with a failing store = %v", err)
	}
	h.store.failSaves(nil)
	// Memory still holds the old copy, so the next capture tries again.
	if err := h.service.CaptureAfterRun(context.Background()); err != nil {
		t.Fatalf("CaptureAfterRun: %v", err)
	}
	h.requireSavedCredential(t, "work", testCredential("w", "t2"))
}

func TestNewAccountVaultWithoutStore(t *testing.T) {
	if vault := NewAccountVault(nil); vault != nil {
		t.Fatalf("NewAccountVault(nil) = %#v, want nil", vault)
	}
}

func TestAccountVaultOpenRejectsIncompleteConfig(t *testing.T) {
	for _, test := range []struct {
		name  string
		clear func(*AccountConfig)
	}{
		{"provider", func(c *AccountConfig) { c.Provider = "" }},
		{"label", func(c *AccountConfig) { c.Label = "" }},
		{"credentials", func(c *AccountConfig) { c.Credentials = nil }},
		{"login", func(c *AccountConfig) { c.Login = nil }},
	} {
		h := newAccountFakes(twoAccounts(), testCredential("w", "t1"))
		config := h.config()
		test.clear(&config)
		if service, err := NewAccountVault(h.store).Open(context.Background(), config); err == nil || service != nil {
			t.Errorf("Open without %s = (%v, %v), want an error", test.name, service, err)
		}
		if h.store.loads != 0 {
			t.Errorf("Open without %s read the store", test.name)
		}
	}
}

func TestAccountVaultOpenReturnsStoreErrors(t *testing.T) {
	h := newAccountFakes(twoAccounts(), testCredential("w", "t1"))
	loadErr := errors.New("corrupt accounts file")
	h.store.loadErr = loadErr

	if _, err := NewAccountVault(h.store).Open(context.Background(), h.config()); !errors.Is(err, loadErr) {
		t.Fatalf("Open with a failing store = %v", err)
	}
	if h.credentials.writeCount() != 0 {
		t.Fatal("Open wrote the host without loading the accounts")
	}
}

func TestAccountVaultOpenReconcilesHostLogin(t *testing.T) {
	for _, test := range []struct {
		name       string
		saved      AccountSet
		host       json.RawMessage
		readErr    error
		wantHost   json.RawMessage
		wantWrites int
	}{
		// The CLI may have refreshed the active account's login after the
		// saved copy was taken; the next capture validates it.
		{name: "same account kept", saved: twoAccounts(), host: testCredential("w", "t5"), wantHost: testCredential("w", "t5")},
		{name: "unchanged login kept", saved: twoAccounts(), host: testCredential("w", "t1"), wantHost: testCredential("w", "t1")},
		{name: "other account replaced", saved: twoAccounts(), host: testCredential("x", "t1"), wantHost: testCredential("w", "t1"), wantWrites: 1},
		{name: "identity-less login replaced", saved: twoAccounts(), host: json.RawMessage(`{"token":"t1"}`), wantHost: testCredential("w", "t1"), wantWrites: 1},
		{name: "signed-out host written", saved: twoAccounts(), wantHost: testCredential("w", "t1"), wantWrites: 1},
		{name: "unreadable host written", saved: twoAccounts(), host: testCredential("w", "t5"), readErr: errors.New("permission denied"), wantHost: testCredential("w", "t1"), wantWrites: 1},
		{name: "no active account leaves host alone", saved: AccountSet{}, host: testCredential("x", "t1"), wantHost: testCredential("x", "t1")},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newAccountFakes(test.saved, test.host)
			h.credentials.readErr = test.readErr
			h.open(t)

			h.requireHost(t, test.wantHost)
			if writes := h.credentials.writeCount(); writes != test.wantWrites {
				t.Fatalf("Open wrote the host %d times, want %d", writes, test.wantWrites)
			}
			h.requireNoSaves(t)
		})
	}
}

func TestAccountVaultOpenSurvivesHostWriteFailure(t *testing.T) {
	h := newAccountFakes(twoAccounts(), testCredential("x", "t1"))
	h.credentials.writeErr = errors.New("read-only file system")
	h.open(t)

	h.requireHost(t, testCredential("x", "t1"))
	release, err := h.service.BeginRun()
	if !errors.Is(err, ErrAccountNotApplied) || release != nil {
		t.Fatalf("BeginRun with a stale host = (%t, %v), want ErrAccountNotApplied", release != nil, err)
	}

	h.credentials.failWrites(nil)
	h.beginRun(t)()
	h.requireHost(t, testCredential("w", "t1"))
}

// withinDeadline fails the test instead of hanging when fn deadlocks.
func withinDeadline(t *testing.T, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not return; Changed probably ran under an account lock", name)
	}
}

func TestAccountServiceChangedRunsWithoutLocks(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	ctx := context.Background()
	var hookErr error
	h.changedHook = func() {
		// Each of these takes an account lock, so they deadlock if Changed
		// is called while one is held.
		h.service.AccountsSnapshot()
		h.service.LoginEnv(nil)
		release, err := h.service.BeginRun()
		if err != nil {
			hookErr = err
			return
		}
		release()
	}

	for _, operation := range []struct {
		name string
		run  func() error
	}{
		{"ImportCurrent", func() error {
			h.credentials.setHost(testCredential("n", "t1"))
			return h.service.ImportCurrent(ctx, "New")
		}},
		{"ActivateAccount", func() error { return h.service.ActivateAccount(ctx, "home") }},
		{"DeleteAccount", func() error { return h.service.DeleteAccount(ctx, "work") }},
		{"refused ActivateAccount", func() error {
			if err := h.service.ActivateAccount(ctx, "missing"); !errors.Is(err, ErrAccountNotFound) {
				return fmt.Errorf("ActivateAccount(missing) = %v", err)
			}
			return nil
		}},
	} {
		changes := h.changes.Load()
		var err error
		withinDeadline(t, operation.name, func() { err = operation.run() })
		if err != nil {
			t.Fatalf("%s: %v", operation.name, err)
		}
		if hookErr != nil {
			t.Fatalf("BeginRun inside Changed after %s: %v", operation.name, hookErr)
		}
		if h.changes.Load() != changes+1 {
			t.Fatalf("%s called Changed %d times, want once", operation.name, h.changes.Load()-changes)
		}
	}

	login := h.startLogin(t, "Other", "")
	login.finishWith(testCredential("o", "t1"), nil)
	changes := h.changes.Load()
	withinDeadline(t, "FinishLogin", func() {
		if handled, err := h.service.FinishLogin(nil, ""); !handled || err != nil {
			t.Errorf("FinishLogin = (%t, %v)", handled, err)
		}
	})
	if h.changes.Load() != changes {
		t.Fatal("FinishLogin called Changed")
	}
}

func TestAccountServiceConcurrentUse(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	ctx := context.Background()
	const iterations = 40
	var wait sync.WaitGroup

	// Runs refresh the active account's login on the host, as a credential
	// sync-back does, then capture it.
	for runner := 0; runner < 3; runner++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := 0; index < iterations; index++ {
				release, err := h.service.BeginRun()
				if err != nil {
					t.Errorf("BeginRun: %v", err)
					return
				}
				id := credentialID(h.credentials.hostCredential())
				h.credentials.setHost(testCredential(id, fmt.Sprintf("r%d-%d", runner, index)))
				release()
				if err := h.service.CaptureAfterRun(ctx); err != nil {
					t.Errorf("CaptureAfterRun: %v", err)
					return
				}
			}
		}()
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		for index := 0; index < iterations; index++ {
			target := []string{"home", "work"}[index%2]
			if err := h.service.ActivateAccount(ctx, target); err != nil && !errors.Is(err, ErrAccountInUse) {
				t.Errorf("ActivateAccount(%s): %v", target, err)
				return
			}
		}
	}()
	wait.Add(1)
	go func() {
		defer wait.Done()
		for index := 0; index < iterations*5; index++ {
			if snapshot := h.service.AccountsSnapshot(); len(snapshot.Items) != 2 {
				t.Errorf("snapshot = %#v", snapshot)
				return
			}
			h.service.LoginEnv(nil)
		}
	}()
	wait.Wait()

	// Every lease was released, memory matches the store, the host holds the
	// active account, and no login crossed accounts.
	if err := h.service.ActivateAccount(ctx, h.service.AccountsSnapshot().ActiveAccountID); err != nil {
		t.Fatalf("ActivateAccount after concurrent use: %v", err)
	}
	saved := h.store.saved()
	h.requireActive(t, saved.ActiveAccountID)
	active, _ := saved.Find(saved.ActiveAccountID)
	h.requireHost(t, active.Credential)
	for id, identity := range map[string]string{"work": "w", "home": "h"} {
		record, _ := saved.Find(id)
		if credentialID(record.Credential) != identity {
			t.Fatalf("account %q holds a login for %q", id, credentialID(record.Credential))
		}
	}
}

// A run that starts after a reconnect of the active account began still holds
// its lease when the login finishes, so the reconnected login is saved but not
// written to the host. It must still reach the host before the next run, and
// the old host login the run leaves behind must not replace it.
func TestAccountServiceReconnectOfActiveAccountDuringRunReachesHost(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	login := h.startLogin(t, "", "work")
	release := h.beginRun(t)
	login.finishWith(testCredential("w", "t9"), nil)
	if handled, err := h.service.FinishLogin(nil, ""); !handled || err != nil {
		t.Fatalf("FinishLogin = (%t, %v)", handled, err)
	}
	h.requireSavedCredential(t, "work", testCredential("w", "t9"))
	release()

	// The old login, which the user reconnected to replace, no longer
	// validates.
	h.credentials.onValidate(func(credential json.RawMessage) (ValidatedAccount, error) {
		if bytes.Equal(credential, testCredential("w", "t1")) {
			return ValidatedAccount{}, errors.New("refresh token revoked")
		}
		return ValidatedAccount{Email: "w@example.com", Credential: credential}, nil
	})
	_ = h.service.CaptureAfterRun(context.Background())
	h.beginRun(t)()

	h.requireSavedCredential(t, "work", testCredential("w", "t9"))
	h.requireHost(t, testCredential("w", "t9"))
}

// Finish undoes host writes a CLI made outside its isolated location. A
// host write the service made during the login, such as an activation, must
// not be silently undone: either the activation is refused while such a login
// is pending, or the host is brought back in line before the next run.
func TestAccountServiceHostWriteUndoneByFailedLoginIsReconciled(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"), loginMayWriteHost)
	h.flow.set(func(f *fakeAccountLoginFlow) { f.undoHostWrites = true })
	login := h.startLogin(t, "New", "")

	if err := h.service.ActivateAccount(context.Background(), "home"); err != nil {
		if errors.Is(err, ErrAccountLoginInProgress) {
			return
		}
		t.Fatalf("ActivateAccount during a login: %v", err)
	}
	h.requireHost(t, testCredential("h", "t1"))
	login.finishWith(nil, errors.New("login cancelled"))
	if handled, _ := h.service.FinishLogin(nil, ""); !handled {
		t.Fatal("FinishLogin did not handle the pending login")
	}

	h.beginRun(t)()
	h.requireActive(t, "home")
	h.requireHost(t, testCredential("h", "t1"))
}

// Reconnecting must sign in as the same provider account, so a label never
// silently changes accounts.
func TestAccountServiceReconnectRejectsAnotherAccount(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	login := h.startLogin(t, "", "home")
	login.finishWith(testCredential("x", "t9"), nil)

	handled, err := h.service.FinishLogin(nil, "")
	if !handled || !errors.Is(err, ErrAccountIdentityMismatch) {
		t.Fatalf("FinishLogin = (%t, %v), want an identity mismatch", handled, err)
	}
	h.requireNoSaves(t)
	h.requireSavedCredential(t, "home", testCredential("h", "t1"))
	h.requireActive(t, "work")
	h.requireHost(t, testCredential("w", "t1"))
}

// Re-selecting the active account keeps a login the CLI refreshed on the host
// instead of writing the older saved copy over it.
func TestAccountServiceReactivatingKeepsRefreshedHostLogin(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("w", "t2"))

	if err := h.service.ActivateAccount(context.Background(), "work"); err != nil {
		t.Fatalf("ActivateAccount: %v", err)
	}
	h.requireSavedCredential(t, "work", testCredential("w", "t2"))
	h.requireHost(t, testCredential("w", "t2"))
}

// A run starts on the active account even when something replaced or
// removed the host login since the last change, such as a manual login in a
// terminal or a partial credential copy.
func TestAccountServiceBeginRunRestoresActiveAccountOnHost(t *testing.T) {
	for _, test := range []struct {
		name string
		host json.RawMessage
	}{
		{name: "another account", host: testCredential("x", "t1")},
		{name: "signed out"},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
			h.credentials.setHost(test.host)
			h.beginRun(t)()
			h.requireHost(t, testCredential("w", "t1"))
		})
	}

	// A refreshed login for the active account is left for capture.
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.setHost(testCredential("w", "t2"))
	writes := h.credentials.writeCount()
	h.beginRun(t)()
	h.requireHost(t, testCredential("w", "t2"))
	if h.credentials.writeCount() != writes {
		t.Fatal("BeginRun rewrote a same-account host login")
	}
}

// While the host has not received the committed login, what it holds is
// older rather than refreshed, so capture writes the committed login instead
// of saving the host's.
func TestAccountServiceCaptureWhileHostStaleWritesCommittedLogin(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.credentials.failWrites(errors.New("disk full"))
	if err := h.service.ActivateAccount(context.Background(), "home"); !errors.Is(err, ErrAccountNotApplied) {
		t.Fatalf("ActivateAccount = %v, want ErrAccountNotApplied", err)
	}
	h.credentials.failWrites(nil)

	if err := h.service.CaptureAfterRun(context.Background()); err != nil {
		t.Fatalf("CaptureAfterRun: %v", err)
	}
	h.requireActive(t, "home")
	h.requireSavedCredential(t, "home", testCredential("h", "t1"))
	h.requireHost(t, testCredential("h", "t1"))
}

// Importing writes the host, so it waits for a login whose CLI may write the
// host itself.
func TestAccountServiceImportCurrentRefusedDuringHostWritingLogin(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"), loginMayWriteHost)
	h.startLogin(t, "New", "")

	if err := h.service.ImportCurrent(context.Background(), "Imported"); !errors.Is(err, ErrAccountLoginInProgress) {
		t.Fatalf("ImportCurrent = %v, want ErrAccountLoginInProgress", err)
	}
	h.requireNoSaves(t)
}

func TestAccountServiceTracksIsolatedRunsPerAccount(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	first := h.service.BeginIsolatedRun("work")
	second := h.service.BeginIsolatedRun("work")
	if h.service.WithIdleIsolatedAccount("work", func() {}) || !h.service.WithIdleIsolatedAccount("home", func() {}) {
		t.Fatal("isolated runs were not tracked per account")
	}
	first()
	first()
	if h.service.WithIdleIsolatedAccount("work", func() {}) {
		t.Fatal("releasing one run twice ended another run of the same account")
	}
	second()
	if !h.service.WithIdleIsolatedAccount("work", func() {}) {
		t.Fatal("the account still reads as running after its runs ended")
	}
}

func TestAccountServiceUsageReadFinishesBeforeAnIsolatedRunStarts(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	reading := make(chan struct{})
	finish := make(chan struct{})
	readDone := make(chan bool, 1)
	go func() {
		readDone <- h.service.WithIdleIsolatedAccount("work", func() {
			close(reading)
			<-finish
		})
	}()
	<-reading
	if h.service.WithIdleIsolatedAccount("work", func() {}) {
		t.Fatal("a second usage read started for the same account")
	}
	if !h.service.WithIdleIsolatedAccount("home", func() {}) {
		t.Fatal("an unrelated account's usage read was blocked")
	}
	started := make(chan func(), 1)
	go func() { started <- h.service.BeginIsolatedRun("work") }()
	select {
	case release := <-started:
		release()
		t.Fatal("a run started before the usage read finished")
	case <-time.After(100 * time.Millisecond):
	}
	close(finish)
	if !<-readDone {
		t.Fatal("the usage read was skipped")
	}
	release := <-started
	if h.service.WithIdleIsolatedAccount("work", func() {}) {
		t.Fatal("usage read started while the run was active")
	}
	release()
	if !h.service.WithIdleIsolatedAccount("work", func() {}) {
		t.Fatal("usage read stayed blocked after the run finished")
	}
}

func TestAccountServiceWithIdleHostLoginSkipsABusyHostLogin(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"), loginMayWriteHost)
	read := 0
	count := func() { read++ }

	release := h.beginRun(t)
	if h.service.WithIdleHostLogin("work", count) || read != 0 {
		t.Fatal("the host login was read while a run held it")
	}
	release()

	h.startLogin(t, "New", "")
	if h.service.WithIdleHostLogin("work", count) || read != 0 {
		t.Fatal("the host login was read while an account login could write it")
	}
	_, _ = h.service.FinishLogin(errors.New("login abandoned"), "")
	if !h.service.WithIdleHostLogin("work", count) || read != 1 {
		t.Fatal("an idle host login was not read")
	}
}

// Account changes wait for a host read instead of rewriting the login under
// the CLI that is reading it.
func TestAccountServiceWithIdleHostLoginHoldsAccountChanges(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	reading := make(chan struct{})
	finish := make(chan struct{})
	readDone := make(chan bool, 1)
	go func() {
		readDone <- h.service.WithIdleHostLogin("work", func() {
			close(reading)
			<-finish
		})
	}()
	<-reading

	activated := make(chan error, 1)
	go func() { activated <- h.service.ActivateAccount(context.Background(), "home") }()
	select {
	case err := <-activated:
		t.Fatalf("ActivateAccount finished during a host read: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	h.requireHost(t, testCredential("w", "t1"))

	close(finish)
	if !<-readDone {
		t.Fatal("an idle host login was not read")
	}
	if err := <-activated; err != nil {
		t.Fatalf("ActivateAccount after the read: %v", err)
	}
	h.requireActive(t, "home")
}
