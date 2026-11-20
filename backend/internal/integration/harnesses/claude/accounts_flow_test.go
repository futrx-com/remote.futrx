package claude

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

// fakeClaudeLogin behaves like `claude auth login --claudeai`: it prints the
// authorization URL, reads the pasted code, and stores the login where the
// real CLI does, preferring CLAUDE_SECURESTORAGE_CONFIG_DIR for tokens. The
// pasted code doubles as the account name unless FAKE_CLAUDE_ACCOUNT names
// the account.
const fakeClaudeLogin = `
if [ "$1 $2" = "auth login" ]; then
  printf 'Opening browser to sign in...\n'
  printf "If the browser didn't open, visit: \033]8;;https://claude.com/cai/oauth/authorize?code=true&state=test\007https://claude.com/cai/oauth/authorize?code=true&state=test\033]8;;\007\n"
  printf 'Paste code here if prompted > '
  if [ -n "$FAKE_CLAUDE_BROWSER_LOGIN" ]; then
    # The browser the CLI opened completed the login via its local callback.
    sleep 0.2
    code="$FAKE_CLAUDE_BROWSER_LOGIN"
  else
    read code
  fi
  if [ "$FAKE_CLAUDE_WRITE" != "none" ]; then
    tokens="${CLAUDE_SECURESTORAGE_CONFIG_DIR:-$CLAUDE_CONFIG_DIR}"
    printf '{"claudeAiOauth":{"refreshToken":"%s"}}' "$code" > "$tokens/.credentials.json"
    account="${FAKE_CLAUDE_ACCOUNT:-$code}"
    printf '{"oauthAccount":{"accountUuid":"uuid-%s","emailAddress":"%s@example.test"}}' "$account" "$account" > "$CLAUDE_CONFIG_DIR/.claude.json"
  fi
  if [ -n "$FAKE_CLAUDE_MESSAGE" ]; then
    printf '\033[31m%s\033[0m\n' "$FAKE_CLAUDE_MESSAGE"
  fi
  exit "${FAKE_CLAUDE_EXIT:-0}"
fi
exit 2
`

type claudeLoginHarness struct {
	auth  *Auth
	store *memoryAccountStore
	host  string
	// tmp is TMPDIR, where account logins create their isolated
	// directories.
	tmp string
}

func newClaudeLoginHarness(t *testing.T) *claudeLoginHarness {
	t.Helper()
	installFakeClaude(t, fakeClaudeLogin)
	for _, name := range []string{"FAKE_CLAUDE_WRITE", "FAKE_CLAUDE_MESSAGE", "FAKE_CLAUDE_EXIT", "FAKE_CLAUDE_BROWSER_LOGIN", "FAKE_CLAUDE_ACCOUNT", "CLAUDE_SECURESTORAGE_CONFIG_DIR"} {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	host := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", host)
	writeHostFiles(t, host,
		`{"mcpOAuth":{"server":"kept"}}`,
		`{"projects":{"/workspace":{}}}`,
	)
	store := &memoryAccountStore{}
	auth := newTestAuth(t, store)
	auth.credentials.validate = func(ctx context.Context, credential json.RawMessage) (agentauth.ValidatedAccount, error) {
		if strings.Contains(string(credential), `"rejected"`) {
			return agentauth.ValidatedAccount{}, errors.New("Claude did not recognize a Claude subscription login")
		}
		return acceptCredential(ctx, credential)
	}
	return &claudeLoginHarness{auth: auth, store: store, host: host, tmp: tmp}
}

// login starts an account login, pastes code, and returns the submit error
// and the isolated directory the login used.
func (h *claudeLoginHarness) login(t *testing.T, code, label, accountID string) (error, string) {
	t.Helper()
	snapshot, err := h.auth.accounts.StartAccountLogin(context.Background(), label, accountID)
	if err != nil {
		t.Fatalf("start account login: %v", err)
	}
	if !snapshot.Active || !snapshot.AwaitingCode || !strings.HasPrefix(snapshot.URL, "https://claude.com/cai/oauth/authorize") {
		t.Fatalf("login snapshot = %#v", snapshot)
	}
	root := h.pendingLoginRoot(t)
	return h.auth.SubmitCode(context.Background(), code), root
}

// loginRoots lists the isolated account-login directories still on disk.
func (h *claudeLoginHarness) loginRoots(t *testing.T) []string {
	t.Helper()
	roots, err := filepath.Glob(filepath.Join(h.tmp, "remote-claude-login-*"))
	if err != nil {
		t.Fatal(err)
	}
	return roots
}

// pendingLoginRoot returns the isolated directory of the one pending account
// login.
func (h *claudeLoginHarness) pendingLoginRoot(t *testing.T) string {
	t.Helper()
	roots := h.loginRoots(t)
	if len(roots) != 1 {
		t.Fatalf("isolated login directories = %v, want exactly one", roots)
	}
	return roots[0]
}

// requireLoginRootsRemoved fails when a finished or replaced account login
// left its isolated directory behind.
func (h *claudeLoginHarness) requireLoginRootsRemoved(t *testing.T) {
	t.Helper()
	if roots := h.loginRoots(t); len(roots) != 0 {
		t.Fatalf("isolated login directories were left behind: %v", roots)
	}
}

func (h *claudeLoginHarness) account(t *testing.T, label string) agentauth.AccountRecord {
	t.Helper()
	for _, record := range h.store.accounts.Accounts {
		if record.Label == label {
			return record
		}
	}
	t.Fatalf("account %q not saved: %#v", label, h.store.accounts)
	return agentauth.AccountRecord{}
}

func TestClaudeAccountLoginWorkflowAddsAndSwitchesAccounts(t *testing.T) {
	h := newClaudeLoginHarness(t)

	err, personalRoot := h.login(t, "personal", "Personal", "")
	if err != nil {
		t.Fatalf("personal login: %v", err)
	}
	err, companyRoot := h.login(t, "company", "Company", "")
	if err != nil {
		t.Fatalf("company login: %v", err)
	}
	if state := h.auth.Status().Login; !state.Completed || state.Error != "" {
		t.Fatalf("login state = %#v", state)
	}

	personal, company := h.account(t, "Personal"), h.account(t, "Company")
	if len(h.store.accounts.Accounts) != 2 || h.store.accounts.ActiveAccountID != company.ID {
		t.Fatalf("saved accounts = %#v", h.store.accounts)
	}
	if personal.Email != "personal@example.test" || company.Email != "company@example.test" {
		t.Fatalf("account emails = %q / %q", personal.Email, company.Email)
	}
	credentials := readFile(t, filepath.Join(h.host, ".credentials.json"))
	config := readFile(t, filepath.Join(h.host, ".claude.json"))
	if !strings.Contains(credentials, `"company"`) || !strings.Contains(credentials, "kept") {
		t.Fatalf("host credentials = %s", credentials)
	}
	if !strings.Contains(config, "uuid-company") || !strings.Contains(config, "/workspace") {
		t.Fatalf("host config = %s", config)
	}
	if personalRoot == companyRoot {
		t.Fatalf("logins shared the isolated directory %s", personalRoot)
	}
	h.requireLoginRootsRemoved(t)

	if err := h.auth.accounts.ActivateAccount(context.Background(), personal.ID); err != nil {
		t.Fatal(err)
	}
	credentials = readFile(t, filepath.Join(h.host, ".credentials.json"))
	config = readFile(t, filepath.Join(h.host, ".claude.json"))
	if !strings.Contains(credentials, `"personal"`) || !strings.Contains(credentials, "kept") ||
		!strings.Contains(config, "uuid-personal") || h.store.accounts.ActiveAccountID != personal.ID {
		t.Fatalf("after switch: credentials = %s, config = %s, active = %q", credentials, config, h.store.accounts.ActiveAccountID)
	}
}

func TestClaudeAccountLoginWithoutCredentialExplainsWhy(t *testing.T) {
	h := newClaudeLoginHarness(t)
	hostBefore := readFile(t, filepath.Join(h.host, ".credentials.json"))
	t.Setenv("FAKE_CLAUDE_WRITE", "none")
	t.Setenv("FAKE_CLAUDE_MESSAGE", "Login successful.")

	err, root := h.login(t, "company", "Company", "")
	if err == nil {
		t.Fatal("login without credentials succeeded")
	}
	for _, want := range []string{
		"Claude login completed without writing credentials to " + filepath.Join(root, ".claude", ".credentials.json"),
		"claude output:",
		"Login successful.",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "\x1b") {
		t.Errorf("error kept terminal escape sequences: %q", err)
	}
	if state := h.auth.Status().Login; state.Error != err.Error() || state.Completed {
		t.Fatalf("login state = %#v", state)
	}
	if len(h.store.accounts.Accounts) != 0 {
		t.Fatalf("failed login saved an account: %#v", h.store.accounts)
	}
	if after := readFile(t, filepath.Join(h.host, ".credentials.json")); after != hostBefore {
		t.Fatalf("failed login changed host credentials: %s", after)
	}
	h.requireLoginRootsRemoved(t)

	t.Setenv("FAKE_CLAUDE_WRITE", "")
	if err, _ := h.login(t, "company", "Company", ""); err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
}

func TestClaudeAccountLoginCommandFailureReportsCLIOutput(t *testing.T) {
	h := newClaudeLoginHarness(t)
	t.Setenv("FAKE_CLAUDE_WRITE", "none")
	t.Setenv("FAKE_CLAUDE_EXIT", "1")
	t.Setenv("FAKE_CLAUDE_MESSAGE", "Login failed: Request failed with status code 400")

	err, _ := h.login(t, "expired-code", "Company", "")
	if err == nil || !strings.HasPrefix(err.Error(), "claude login failed: exit status 1") ||
		!strings.Contains(err.Error(), "status code 400") {
		t.Fatalf("error = %v", err)
	}
	if len(h.store.accounts.Accounts) != 0 {
		t.Fatalf("failed login saved an account: %#v", h.store.accounts)
	}
}

func TestClaudeAccountLoginRejectedByValidationSavesNothing(t *testing.T) {
	h := newClaudeLoginHarness(t)
	hostBefore := readFile(t, filepath.Join(h.host, ".credentials.json"))

	err, _ := h.login(t, "rejected", "Company", "")
	if err == nil || err.Error() != "Claude did not recognize a Claude subscription login" {
		t.Fatalf("error = %v", err)
	}
	if len(h.store.accounts.Accounts) != 0 {
		t.Fatalf("rejected login saved an account: %#v", h.store.accounts)
	}
	if after := readFile(t, filepath.Join(h.host, ".credentials.json")); after != hostBefore {
		t.Fatalf("rejected login changed host credentials: %s", after)
	}
	h.requireLoginRootsRemoved(t)
}

// The CLI stores tokens under CLAUDE_SECURESTORAGE_CONFIG_DIR when set. An
// inherited value must not pull the new login out of the private directory.
func TestClaudeAccountLoginIgnoresInheritedSecureStorageDir(t *testing.T) {
	h := newClaudeLoginHarness(t)
	secureStorage := t.TempDir()
	t.Setenv("CLAUDE_SECURESTORAGE_CONFIG_DIR", secureStorage)

	if err, _ := h.login(t, "company", "Company", ""); err != nil {
		t.Fatalf("login: %v", err)
	}
	if company := h.account(t, "Company"); !strings.Contains(string(company.Credential), `"company"`) {
		t.Fatalf("saved credential = %s", company.Credential)
	}
	if _, err := os.Stat(filepath.Join(secureStorage, ".credentials.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("login wrote the inherited secure-storage directory: %v", err)
	}
}

func TestClaudeAccountLoginReconnectKeepsLabel(t *testing.T) {
	h := newClaudeLoginHarness(t)
	if err, _ := h.login(t, "first", "Personal", ""); err != nil {
		t.Fatal(err)
	}
	original := h.account(t, "Personal")

	// The same account signs in again with new tokens.
	t.Setenv("FAKE_CLAUDE_ACCOUNT", "first")
	if err, _ := h.login(t, "second", "", original.ID); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	updated := h.account(t, "Personal")
	if len(h.store.accounts.Accounts) != 1 || updated.ID != original.ID ||
		!strings.Contains(string(updated.Credential), `"second"`) {
		t.Fatalf("saved accounts = %#v", h.store.accounts)
	}
}

// Reconnecting a saved account must not quietly point its label at another
// Claude account.
func TestClaudeAccountLoginReconnectRejectsAnotherAccount(t *testing.T) {
	h := newClaudeLoginHarness(t)
	if err, _ := h.login(t, "first", "Personal", ""); err != nil {
		t.Fatal(err)
	}
	original := h.account(t, "Personal")

	err, _ := h.login(t, "other", "", original.ID)
	if err == nil || !strings.Contains(err.Error(), "add it as a new account instead") {
		t.Fatalf("reconnect as another account: %v", err)
	}
	if state := h.auth.Status().Login; state.Completed || !strings.Contains(state.Error, "not saved Claude account") {
		t.Fatalf("login state = %#v", state)
	}
	if updated := h.account(t, "Personal"); len(h.store.accounts.Accounts) != 1 || string(updated.Credential) != string(original.Credential) {
		t.Fatalf("saved accounts = %#v", h.store.accounts)
	}
	if credentials := readFile(t, filepath.Join(h.host, ".credentials.json")); !strings.Contains(credentials, `"first"`) {
		t.Fatalf("host credentials = %s", credentials)
	}
}

func TestClaudeAccountLoginDuringRunSavesWithoutActivating(t *testing.T) {
	h := newClaudeLoginHarness(t)
	if err, _ := h.login(t, "personal", "Personal", ""); err != nil {
		t.Fatal(err)
	}
	personal := h.account(t, "Personal")
	done, err := h.auth.accounts.BeginRun()
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	if _, err := h.auth.accounts.StartAccountLogin(context.Background(), "Company", ""); !errors.Is(err, agentauth.ErrAccountInUse) {
		t.Fatalf("login during a run: %v", err)
	}
	h.requireLoginRootsRemoved(t)
	done()

	// A run that starts after the login began must not have its account
	// switched underneath it when the login completes.
	if _, err := h.auth.accounts.StartAccountLogin(context.Background(), "Company", ""); err != nil {
		t.Fatal(err)
	}
	release, err := h.auth.accounts.BeginRun()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := h.auth.SubmitCode(context.Background(), "company"); err != nil {
		t.Fatal(err)
	}
	h.account(t, "Company")
	if h.store.accounts.ActiveAccountID != personal.ID {
		t.Fatalf("active account switched during a run: %q", h.store.accounts.ActiveAccountID)
	}
	if credentials := readFile(t, filepath.Join(h.host, ".credentials.json")); !strings.Contains(credentials, `"personal"`) {
		t.Fatalf("host credentials switched during a run: %s", credentials)
	}
}

func TestClaudeStartAccountLoginReplacesAbandonedLogin(t *testing.T) {
	h := newClaudeLoginHarness(t)
	if _, err := h.auth.accounts.StartAccountLogin(context.Background(), "Abandoned", ""); err != nil {
		t.Fatal(err)
	}
	abandonedRoot := h.pendingLoginRoot(t)

	// login requires exactly one isolated directory, so the abandoned one
	// must already be gone when the replacement starts.
	err, root := h.login(t, "company", "Company", "")
	if err != nil {
		t.Fatalf("replacement login: %v", err)
	}
	if root == abandonedRoot {
		t.Fatalf("replacement login reused the abandoned directory %s", root)
	}
	h.requireLoginRootsRemoved(t)
	if len(h.store.accounts.Accounts) != 1 || h.store.accounts.Accounts[0].Label != "Company" {
		t.Fatalf("saved accounts = %#v", h.store.accounts)
	}
}

func TestIsolatedClaudeAuthEnvRemovesSecureStorageOverride(t *testing.T) {
	env := isolatedClaudeAuthEnvFor([]string{
		"PATH=/usr/bin",
		"CLAUDE_SECURESTORAGE_CONFIG_DIR=/root/.claude",
		"CLAUDE_CONFIG_DIR=/root/.claude",
	}, "/tmp/private")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "CLAUDE_SECURESTORAGE_CONFIG_DIR") || !strings.Contains(joined, "CLAUDE_CONFIG_DIR=/tmp/private") ||
		!strings.Contains(joined, "PATH=/usr/bin") {
		t.Fatalf("env = %v", env)
	}
}

// Signing in through the browser the CLI opened finishes the login without a
// pasted code. The account must still be saved, and a code pasted afterwards
// must not fail on the closed terminal.
func TestClaudeAccountLoginCompletedInBrowserIsSavedWithoutCode(t *testing.T) {
	h := newClaudeLoginHarness(t)
	t.Setenv("FAKE_CLAUDE_BROWSER_LOGIN", "company")
	t.Setenv("FAKE_CLAUDE_MESSAGE", "Login successful.")

	if _, err := h.auth.accounts.StartAccountLogin(context.Background(), "Company", ""); err != nil {
		t.Fatal(err)
	}
	h.pendingLoginRoot(t)
	deadline := time.Now().Add(5 * time.Second)
	for h.auth.Status().Login.Active {
		if time.Now().After(deadline) {
			t.Fatal("browser login did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if state := h.auth.Status().Login; !state.Completed || state.Error != "" {
		t.Fatalf("login state = %#v", state)
	}
	company := h.account(t, "Company")
	if h.store.accounts.ActiveAccountID != company.ID || company.Email != "company@example.test" {
		t.Fatalf("saved accounts = %#v", h.store.accounts)
	}
	if credentials := readFile(t, filepath.Join(h.host, ".credentials.json")); !strings.Contains(credentials, `"company"`) {
		t.Fatalf("host credentials = %s", credentials)
	}
	h.requireLoginRootsRemoved(t)
	if err := h.auth.SubmitCode(context.Background(), "pasted-too-late"); err != nil {
		t.Fatalf("code pasted after the browser login: %v", err)
	}
	if len(h.store.accounts.Accounts) != 1 {
		t.Fatalf("late code saved another account: %#v", h.store.accounts)
	}
}
