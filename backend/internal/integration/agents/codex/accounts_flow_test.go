package codex

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

// fakeCodexLogin prints the device prompt the way the real CLI does and then
// writes a login for ChatGPT account acct-$FAKE_CODEX_ACCOUNT (default
// $FAKE_CODEX_TOKEN) to FAKE_CODEX_TARGET, or to the isolated CODEX_HOME when
// no target is given. FAKE_CODEX_TARGET=none writes nothing. The CODEX_HOME
// it was given is recorded in FAKE_CODEX_HOME_RECORD.
const fakeCodexLogin = `
printf '%s' "$CODEX_HOME" > "$FAKE_CODEX_HOME_RECORD"
printf '\033[94mhttps://auth.openai.com/codex/device\033[0m\n'
printf '\033[94mABCD-12345\033[0m\n'
if [ -n "$FAKE_CODEX_WAIT_FOR" ]; then
  while [ ! -e "$FAKE_CODEX_WAIT_FOR" ]; do sleep 0.02; done
fi
target="${FAKE_CODEX_TARGET:-$CODEX_HOME/auth.json}"
if [ "$target" != "none" ]; then
  mkdir -p "$(dirname "$target")"
  printf '{"auth_mode":"chatgpt","tokens":{"account_id":"acct-%s","access_token":"%s"}}' \
    "${FAKE_CODEX_ACCOUNT:-$FAKE_CODEX_TOKEN}" "$FAKE_CODEX_TOKEN" > "$target"
fi
if [ -n "$FAKE_CODEX_MESSAGE" ]; then
  printf '%s\n' "$FAKE_CODEX_MESSAGE"
fi
exit "${FAKE_CODEX_EXIT:-0}"
`

type codexLoginHarness struct {
	auth           *Auth
	store          *memoryAccountStore
	activePath     string
	homeRecord     string
	validatedCalls atomic.Int32
}

// newCodexLoginHarness installs a fake codex first on PATH, points the host
// credential at a temporary CODEX_HOME, and validates credentials by reading
// the account back so each account gets a distinct email. A login whose
// access token is "rejected" fails validation.
func newCodexLoginHarness(t *testing.T) *codexLoginHarness {
	t.Helper()
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\n"+fakeCodexLogin), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", filepath.Join(t.TempDir(), "host-home"))
	canonicalHome := filepath.Join(t.TempDir(), ".codex")
	t.Setenv("CODEX_HOME", canonicalHome)
	for _, name := range []string{"FAKE_CODEX_TARGET", "FAKE_CODEX_TOKEN", "FAKE_CODEX_ACCOUNT", "FAKE_CODEX_MESSAGE", "FAKE_CODEX_EXIT", "FAKE_CODEX_WAIT_FOR"} {
		t.Setenv(name, "")
	}
	homeRecord := filepath.Join(t.TempDir(), "codex-home")
	t.Setenv("FAKE_CODEX_HOME_RECORD", homeRecord)

	harness := &codexLoginHarness{
		store:      &memoryAccountStore{},
		activePath: filepath.Join(canonicalHome, "auth.json"),
		homeRecord: homeRecord,
	}
	harness.auth = newTestAuth(t, harness.store)
	harness.auth.credentials.validate = func(_ context.Context, credential json.RawMessage) (agentauth.ValidatedAccount, error) {
		harness.validatedCalls.Add(1)
		var parsed struct {
			Tokens struct {
				AccountID   string `json:"account_id"`
				AccessToken string `json:"access_token"`
			} `json:"tokens"`
		}
		if err := json.Unmarshal(credential, &parsed); err != nil || parsed.Tokens.AccountID == "" ||
			parsed.Tokens.AccessToken == "rejected" {
			return agentauth.ValidatedAccount{}, errors.New("Codex did not recognize a ChatGPT account")
		}
		return agentauth.ValidatedAccount{
			Email: strings.TrimPrefix(parsed.Tokens.AccountID, "acct-") + "@example.test", PlanType: "plus",
			Credential: append(json.RawMessage(nil), credential...),
		}, nil
	}
	return harness
}

// login runs one account login to completion and returns its final state and
// the isolated directory it used.
func (h *codexLoginHarness) login(t *testing.T, token, label, accountID string) (agentauth.DeviceState, string) {
	t.Helper()
	t.Setenv("FAKE_CODEX_TOKEN", token)
	if err := os.Remove(h.homeRecord); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	snapshot, err := h.auth.accounts.StartAccountLogin(context.Background(), label, accountID)
	if err != nil {
		t.Fatalf("start account login: %v", err)
	}
	if snapshot.URL != "https://auth.openai.com/codex/device" || snapshot.UserCode != "ABCD-12345" {
		t.Fatalf("login snapshot = %#v", snapshot)
	}
	state := waitForCodexLogin(t, h.auth)
	home, err := os.ReadFile(h.homeRecord)
	if err != nil {
		t.Fatal(err)
	}
	return state, filepath.Dir(string(home))
}

func waitForCodexLogin(t *testing.T, auth *Auth) agentauth.DeviceState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for auth.LoginState().Active {
		if time.Now().After(deadline) {
			t.Fatal("Codex login did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return auth.LoginState()
}

func (h *codexLoginHarness) activeCredential(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(h.activePath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (h *codexLoginHarness) account(t *testing.T, label string) agentauth.AccountRecord {
	t.Helper()
	saved := h.store.saved()
	for _, record := range saved.Accounts {
		if record.Label == label {
			return record
		}
	}
	t.Fatalf("account %q not saved: %#v", label, saved)
	return agentauth.AccountRecord{}
}

func TestAccountLoginWorkflowAddsAndSwitchesAccounts(t *testing.T) {
	h := newCodexLoginHarness(t)

	state, personalRoot := h.login(t, "personal", "Personal", "")
	if !state.Completed || state.Error != "" {
		t.Fatalf("personal login = %#v", state)
	}
	state, companyRoot := h.login(t, "company", "Company", "")
	if !state.Completed || state.Error != "" {
		t.Fatalf("company login = %#v", state)
	}

	personal, company := h.account(t, "Personal"), h.account(t, "Company")
	saved := h.store.saved()
	if len(saved.Accounts) != 2 || personal.ID == company.ID {
		t.Fatalf("saved accounts = %#v", saved)
	}
	if saved.ActiveAccountID != company.ID {
		t.Fatalf("active account = %q, want the newest login %q", saved.ActiveAccountID, company.ID)
	}
	if personal.Email != "personal@example.test" || company.Email != "company@example.test" || company.PlanType != "plus" {
		t.Fatalf("account metadata = %#v / %#v", personal, company)
	}
	if !strings.Contains(string(personal.Credential), `"acct-personal"`) || !strings.Contains(string(company.Credential), `"acct-company"`) {
		t.Fatalf("saved credentials = %s / %s", personal.Credential, company.Credential)
	}
	if active := h.activeCredential(t); active != string(company.Credential) {
		t.Fatalf("host credential = %s, want the company credential", active)
	}
	for _, root := range []string{personalRoot, companyRoot} {
		requireRemoved(t, root)
	}

	snapshot := h.auth.accounts.AccountsSnapshot()
	if len(snapshot.Items) != 2 || snapshot.ActiveAccountID != company.ID {
		t.Fatalf("accounts snapshot = %#v", snapshot)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "tokens") || strings.Contains(string(encoded), "credential") {
		t.Fatalf("accounts snapshot leaked a credential: %s", encoded)
	}

	if err := h.auth.accounts.ActivateAccount(context.Background(), personal.ID); err != nil {
		t.Fatal(err)
	}
	if active := h.store.saved().ActiveAccountID; active != personal.ID {
		t.Fatalf("active account after switch = %q", active)
	}
	if active := h.activeCredential(t); active != string(personal.Credential) {
		t.Fatalf("host credential after switch = %s", active)
	}
}

func TestAccountLoginReconnectUpdatesTheSameAccount(t *testing.T) {
	h := newCodexLoginHarness(t)
	if state, _ := h.login(t, "first", "Personal", ""); !state.Completed {
		t.Fatalf("first login = %#v", state)
	}
	original := h.account(t, "Personal")

	// Signing in again to the same ChatGPT account issues new tokens.
	t.Setenv("FAKE_CODEX_ACCOUNT", "first")
	state, _ := h.login(t, "second", "", original.ID)
	if !state.Completed || state.Error != "" {
		t.Fatalf("reconnect = %#v", state)
	}
	if saved := h.store.saved(); len(saved.Accounts) != 1 {
		t.Fatalf("reconnect created another account: %#v", saved)
	}
	updated := h.account(t, "Personal")
	if updated.ID != original.ID || !strings.Contains(string(updated.Credential), `"second"`) {
		t.Fatalf("reconnected account = %#v", updated)
	}
	if active := h.activeCredential(t); active != string(updated.Credential) {
		t.Fatalf("host credential = %s", active)
	}
}

func TestAccountLoginWithoutCredentialExplainsWhereItLooked(t *testing.T) {
	h := newCodexLoginHarness(t)
	if state, _ := h.login(t, "personal", "Personal", ""); !state.Completed {
		t.Fatalf("setup login = %#v", state)
	}
	before := h.activeCredential(t)
	saved := h.store.saved()

	t.Setenv("FAKE_CODEX_TARGET", "none")
	t.Setenv("FAKE_CODEX_MESSAGE", "Error: device code expired before it was authorized")
	state, root := h.login(t, "company", "Company", "")
	if state.Completed {
		t.Fatalf("login without credentials completed: %#v", state)
	}
	for _, want := range []string{
		"Codex login completed without writing credentials",
		filepath.Join(root, ".codex", "auth.json"),
		h.activePath,
		"codex output:",
		"Error: device code expired before it was authorized",
	} {
		if !strings.Contains(state.Error, want) {
			t.Errorf("error %q does not mention %q", state.Error, want)
		}
	}
	if strings.Contains(state.Error, "\x1b") {
		t.Errorf("error kept terminal escape sequences: %q", state.Error)
	}
	requireRemoved(t, root)
	if after := h.store.saved(); len(after.Accounts) != len(saved.Accounts) || after.ActiveAccountID != saved.ActiveAccountID {
		t.Fatalf("failed login changed saved accounts: %#v", after)
	}
	if after := h.activeCredential(t); after != before {
		t.Fatalf("failed login changed the host credential: %s", after)
	}

	// The failed attempt must not block the next one.
	t.Setenv("FAKE_CODEX_TARGET", "")
	t.Setenv("FAKE_CODEX_MESSAGE", "")
	if state, _ := h.login(t, "company", "Company", ""); !state.Completed || state.Error != "" {
		t.Fatalf("retry after failure = %#v", state)
	}
}

func TestAccountLoginCommandFailureSavesNothing(t *testing.T) {
	h := newCodexLoginHarness(t)
	t.Setenv("FAKE_CODEX_TARGET", "none")
	t.Setenv("FAKE_CODEX_EXIT", "1")
	t.Setenv("FAKE_CODEX_MESSAGE", "Error logging in: access denied")

	state, _ := h.login(t, "company", "Company", "")
	if state.Completed || !strings.HasPrefix(state.Error, "codex login failed: exit status 1") ||
		!strings.Contains(state.Error, "access denied") {
		t.Fatalf("login state = %#v", state)
	}
	if saved := h.store.saved(); len(saved.Accounts) != 0 || h.validatedCalls.Load() != 0 {
		t.Fatalf("failed command saved or validated an account: %#v", saved)
	}
	if _, err := os.Stat(h.activePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed command wrote a host credential: %v", err)
	}
}

// A Codex build that pins CODEX_HOME (for example a Snap) writes the host
// credential instead of the isolated one. The login must still be captured,
// and the previously active account must stay intact in the vault.
func TestAccountLoginCapturesCodexThatIgnoresIsolatedHome(t *testing.T) {
	h := newCodexLoginHarness(t)
	if state, _ := h.login(t, "personal", "Personal", ""); !state.Completed {
		t.Fatalf("setup login = %#v", state)
	}
	personal := h.account(t, "Personal")

	t.Setenv("FAKE_CODEX_TARGET", h.activePath)
	state, _ := h.login(t, "company", "Company", "")
	if !state.Completed || state.Error != "" {
		t.Fatalf("login state = %#v", state)
	}
	company := h.account(t, "Company")
	if saved := h.store.saved(); saved.ActiveAccountID != company.ID || !strings.Contains(string(company.Credential), `"acct-company"`) {
		t.Fatalf("saved accounts = %#v", saved)
	}
	if kept := h.account(t, "Personal"); string(kept.Credential) != string(personal.Credential) {
		t.Fatalf("personal credential changed to %s", kept.Credential)
	}
	if active := h.activeCredential(t); active != string(company.Credential) {
		t.Fatalf("host credential = %s", active)
	}
}

func TestRejectedLoginRestoresHostCredentialWrittenByCodex(t *testing.T) {
	h := newCodexLoginHarness(t)
	if state, _ := h.login(t, "personal", "Personal", ""); !state.Completed {
		t.Fatalf("setup login = %#v", state)
	}
	before := h.activeCredential(t)

	t.Setenv("FAKE_CODEX_TARGET", h.activePath)
	state, _ := h.login(t, "rejected", "Company", "")
	if state.Completed || state.Error != "Codex did not recognize a ChatGPT account" {
		t.Fatalf("login state = %#v", state)
	}
	if after := h.activeCredential(t); after != before {
		t.Fatalf("host credential = %s, want the previous %s", after, before)
	}
	if saved := h.store.saved(); len(saved.Accounts) != 1 {
		t.Fatalf("rejected login saved an account: %#v", saved)
	}
}

func TestAccountLoginHoldsRunsAndOtherLoginsUntilItFinishes(t *testing.T) {
	h := newCodexLoginHarness(t)
	release := filepath.Join(t.TempDir(), "release")
	t.Setenv("FAKE_CODEX_WAIT_FOR", release)
	t.Setenv("FAKE_CODEX_TOKEN", "company")

	if _, err := h.auth.accounts.StartAccountLogin(context.Background(), "Company", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := h.auth.accounts.BeginRun(); !errors.Is(err, agentauth.ErrAccountLoginInProgress) {
		t.Fatalf("run during an account login error = %v", err)
	}
	if _, err := h.auth.accounts.StartAccountLogin(context.Background(), "Other", ""); err == nil {
		t.Fatal("a second account login started while one was in progress")
	}

	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if state := waitForCodexLogin(t, h.auth); !state.Completed || state.Error != "" {
		t.Fatalf("login state = %#v", state)
	}
	done, err := h.auth.accounts.BeginRun()
	if err != nil {
		t.Fatalf("run after login: %v", err)
	}
	done()
}

func TestAccountLoginRejectsInvalidRequestsBeforeStartingCodex(t *testing.T) {
	h := newCodexLoginHarness(t)
	if state, _ := h.login(t, "personal", "Personal", ""); !state.Completed {
		t.Fatalf("setup login = %#v", state)
	}
	cases := []struct {
		name, label, accountID string
		want                   error
	}{
		{"blank label", "  ", "", agentauth.ErrAccountLabelRequired},
		{"duplicate label", "personal", "", agentauth.ErrAccountLabelConflict},
		{"unknown account", "Other", "missing", agentauth.ErrAccountNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.auth.accounts.StartAccountLogin(context.Background(), tc.label, tc.accountID); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if _, pending := h.auth.accounts.LoginEnv(nil); pending || h.auth.LoginState().Active {
				t.Fatal("a rejected request left a login running")
			}
		})
	}
}

func TestSnapCommandNameFollowsSymlinksIntoSnapBin(t *testing.T) {
	root := t.TempDir()
	previous := snapBinDir
	snapBinDir = filepath.Join(root, "snap", "bin")
	t.Cleanup(func() { snapBinDir = previous })
	for _, dir := range []string{snapBinDir, filepath.Join(root, "usr", "local", "bin"), filepath.Join(root, "opt")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	snapCommand := filepath.Join(snapBinDir, "codex")
	if err := os.Symlink("/usr/bin/snap", snapCommand); err != nil {
		t.Fatal(err)
	}
	absoluteLink := filepath.Join(root, "usr", "local", "bin", "codex")
	if err := os.Symlink(snapCommand, absoluteLink); err != nil {
		t.Fatal(err)
	}
	relativeLink := filepath.Join(root, "opt", "codex")
	if err := os.Symlink("../usr/local/bin/codex", relativeLink); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(root, "opt", "plain-codex")
	if err := os.WriteFile(plain, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	loopA, loopB := filepath.Join(root, "opt", "loop-a"), filepath.Join(root, "opt", "loop-b")
	if err := os.Symlink(loopB, loopA); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(loopA, loopB); err != nil {
		t.Fatal(err)
	}

	for _, binary := range []string{snapCommand, absoluteLink, relativeLink} {
		path, ok := snapCredentialPathFor(binary, "/home/person")
		if !ok || path != "/home/person/snap/codex/current/auth.json" {
			t.Errorf("snapCredentialPathFor(%s) = %q, %v", binary, path, ok)
		}
	}
	for _, binary := range []string{plain, loopA, filepath.Join(root, "missing")} {
		if path, ok := snapCredentialPathFor(binary, "/home/person"); ok {
			t.Errorf("snapCredentialPathFor(%s) = %q, want no Snap", binary, path)
		}
	}
}
