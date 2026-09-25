package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

func TestParseClaudeAuthStatusReadsSubscriptionMetadata(t *testing.T) {
	status, err := parseClaudeAuthStatus([]byte("notice\n" +
		`{"loggedIn":true,"authMethod":"claude.ai","email":"person@example.test","subscriptionType":"max"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !status.LoggedIn || status.AuthMethod != "claude.ai" || status.Email != "person@example.test" || status.SubscriptionType != "max" {
		t.Fatalf("status = %#v", status)
	}
}

func TestCheckOAuthUsableRejectsExpiredLoginWithoutRefreshToken(t *testing.T) {
	now := time.UnixMilli(2_000_000)
	for _, test := range []struct {
		oauth string
		ok    bool
	}{
		{oauth: `{"refreshToken":"r","expiresAt":1}`, ok: true},
		{oauth: `{"accessToken":"a","expiresAt":3000000}`, ok: true},
		{oauth: `{"accessToken":"a","expiresAt":1000000}`, ok: false},
		{oauth: `{}`, ok: false},
	} {
		if err := checkOAuthUsable(json.RawMessage(test.oauth), now); (err == nil) != test.ok {
			t.Fatalf("checkOAuthUsable(%s) = %v, want ok=%v", test.oauth, err, test.ok)
		}
	}
}

func TestIsolatedClaudeAuthEnvOverridesInheritedCredentials(t *testing.T) {
	env := isolatedClaudeAuthEnvFor([]string{
		"HOME=/root",
		"CLAUDE_CONFIG_DIR=/root/.claude",
		"ANTHROPIC_API_KEY=sk-test",
		"CLAUDE_CODE_OAUTH_TOKEN=token",
	}, "/tmp/isolated-claude")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "CLAUDE_CONFIG_DIR=/root/.claude") || !strings.Contains(joined, "CLAUDE_CONFIG_DIR=/tmp/isolated-claude") {
		t.Fatalf("isolated auth env = %#v", env)
	}
	if strings.Contains(joined, "ANTHROPIC_API_KEY=") || strings.Contains(joined, "CLAUDE_CODE_OAUTH_TOKEN=") {
		t.Fatalf("inherited credential leaked into isolated auth env: %#v", env)
	}
}

func TestValidateAccountCredentialKeepsTokensRefreshedByStatus(t *testing.T) {
	installFakeClaude(t, `
if [ "$1 $2" = "auth status" ]; then
  grep -q '"refreshToken": "old"' "$CLAUDE_CONFIG_DIR/.credentials.json" || exit 1
  printf '%s' '{"claudeAiOauth":{"refreshToken":"rotated"}}' > "$CLAUDE_CONFIG_DIR/.credentials.json"
  printf '%s\n' '{"loggedIn":true,"authMethod":"claude.ai","subscriptionType":"pro"}'
fi
`)
	validated, err := validateAccountCredential(context.Background(), testClaudeCredential("old", "uuid-1", "person@example.test"))
	if err != nil {
		t.Fatal(err)
	}
	if validated.Email != "person@example.test" || validated.PlanType != "pro" {
		t.Fatalf("validated = %#v", validated)
	}
	if !strings.Contains(string(validated.Credential), "rotated") || !strings.Contains(string(validated.Credential), "uuid-1") {
		t.Fatalf("validated credential = %s", validated.Credential)
	}
}

func TestValidateAccountCredentialRejectsSignedOutStatus(t *testing.T) {
	installFakeClaude(t, `
printf '%s\n' '{"loggedIn":false,"authMethod":"none"}'
exit 1
`)
	if _, err := validateAccountCredential(context.Background(), testClaudeCredential("old", "uuid-1", "")); err == nil {
		t.Fatal("signed-out credential was accepted")
	}
}

func TestAccountIdentityReadsOAuthAccount(t *testing.T) {
	for _, test := range []struct {
		name       string
		credential string
		want       agentauth.AccountIdentity
	}{
		{
			// The email can change, so the UUID alone names the account.
			name:       "account and email",
			credential: `{"credentials":{},"oauthAccount":{"accountUuid":"uuid-1","emailAddress":"person@example.test"}}`,
			want:       agentauth.AccountIdentity{"account": "uuid-1"},
		},
		{
			name:       "email only",
			credential: `{"oauthAccount":{"emailAddress":" Person@Example.TEST "}}`,
			want:       agentauth.AccountIdentity{"email": "person@example.test"},
		},
		{name: "no profile", credential: `{"credentials":{"claudeAiOauth":{}}}`, want: agentauth.AccountIdentity{}},
		{name: "malformed profile", credential: `{"oauthAccount":"person"}`, want: agentauth.AccountIdentity{}},
		{name: "not JSON", credential: `not json`, want: agentauth.AccountIdentity{}},
	} {
		if got := accountIdentity([]byte(test.credential)); !maps.Equal(got, test.want) {
			t.Errorf("%s: accountIdentity = %#v, want %#v", test.name, got, test.want)
		}
	}

	saved := accountIdentity(testClaudeCredential("saved", "uuid-1", "person@example.test"))
	if !saved.Same(accountIdentity(testClaudeCredential("refreshed", "uuid-1", ""))) {
		t.Error("a refreshed login without an email no longer matches its account")
	}
	if !saved.Same(accountIdentity(testClaudeCredential("renamed", "uuid-1", "new@example.test"))) {
		t.Error("a changed email turned the login into another account")
	}
	if saved.Same(accountIdentity(testClaudeCredential("other", "uuid-2", "person@example.test"))) {
		t.Error("another account with the same email matched")
	}
}

// If the config file cannot be written after the credentials file was, the
// credentials are restored so the host never mixes two accounts, and a failed
// restore is reported rather than discarded.
func TestWriteAccountCredentialRestoresCredentialsWhenConfigWriteFails(t *testing.T) {
	for _, test := range []struct {
		name         string
		restoreFails bool
	}{
		{name: "restored"},
		{name: "restore fails", restoreFails: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			previous := `{"claudeAiOauth":{"refreshToken":"old"},"mcpOAuth":{"server":"kept"}}`
			writeHostFiles(t, dir, previous, `{"oauthAccount":{"accountUuid":"uuid-old"}}`)
			// The first write installs the new credentials, the second is
			// the config file, and the third restores the credentials.
			writes := 0
			failCredentialWrites(t, func(string) bool {
				writes++
				return writes == 2 || (writes == 3 && test.restoreFails)
			})

			err := writeAccountCredential(
				filepath.Join(dir, ".credentials.json"),
				filepath.Join(dir, ".claude.json"),
				testClaudeCredential("new", "uuid-new", ""),
			)
			if err == nil || !strings.Contains(err.Error(), "write .claude.json: disk full") {
				t.Fatalf("error = %v", err)
			}
			restoreFailure := "restore previous Claude credentials: write .credentials.json: disk full"
			if strings.Contains(err.Error(), restoreFailure) != test.restoreFails {
				t.Fatalf("error = %q, want restore failure reported: %v", err, test.restoreFails)
			}
			credentials := readFile(t, filepath.Join(dir, ".credentials.json"))
			if (credentials == previous) == test.restoreFails {
				t.Fatalf("host credentials = %s", credentials)
			}
			if config := readFile(t, filepath.Join(dir, ".claude.json")); !strings.Contains(config, "uuid-old") {
				t.Fatalf("host config = %s", config)
			}
		})
	}
}

func TestAccountLoginCapturesCredentialWrittenToIsolatedConfig(t *testing.T) {
	installFakeClaude(t, `
if [ "$1 $2" = "auth login" ]; then
  printf '%s\n' 'https://claude.com/cai/oauth/authorize?code=true&state=test'
  read code
  printf '%s' '{"claudeAiOauth":{"refreshToken":"new"}}' > "$CLAUDE_CONFIG_DIR/.credentials.json"
  printf '%s' '{"oauthAccount":{"accountUuid":"uuid-new","emailAddress":"new@example.test"}}' > "$CLAUDE_CONFIG_DIR/.claude.json"
fi
`)
	host := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", host)
	writeHostFiles(t, host,
		`{"claudeAiOauth":{"refreshToken":"old"},"mcpOAuth":{"server":"kept"}}`,
		`{"oauthAccount":{"accountUuid":"uuid-old"},"projects":{"/workspace":{}}}`,
	)

	store := &memoryAccountStore{}
	auth := newTestAuth(t, store)
	login, err := auth.accounts.StartAccountLogin(context.Background(), "Company", "")
	if err != nil {
		t.Fatal(err)
	}
	if !login.Active || !login.AwaitingCode || !strings.HasPrefix(login.URL, "https://claude.com/cai/oauth/authorize") {
		t.Fatalf("login = %#v", login)
	}
	if err := auth.SubmitCode(context.Background(), "pasted-code"); err != nil {
		t.Fatal(err)
	}
	if state := auth.Status().Login; !state.Completed || state.Error != "" {
		t.Fatalf("login state = %#v", state)
	}
	if len(store.accounts.Accounts) != 1 || store.accounts.ActiveAccountID != store.accounts.Accounts[0].ID {
		t.Fatalf("saved accounts = %#v", store.accounts)
	}
	credentials := readFile(t, filepath.Join(host, ".credentials.json"))
	config := readFile(t, filepath.Join(host, ".claude.json"))
	if !strings.Contains(credentials, `"new"`) || !strings.Contains(credentials, "kept") {
		t.Fatalf("host credentials = %s", credentials)
	}
	if !strings.Contains(config, "uuid-new") || !strings.Contains(config, "/workspace") {
		t.Fatalf("host config = %s", config)
	}
}

func TestActivateAccountValidatesBeforeReplacingCurrentCredential(t *testing.T) {
	host := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", host)
	oldCredential := testClaudeCredential("old", "uuid-old", "old@example.test")
	newCredential := testClaudeCredential("new", "uuid-new", "new@example.test")
	writeHostFiles(t, host,
		`{"claudeAiOauth":{"refreshToken":"old"},"mcpOAuth":{"server":"kept"}}`,
		`{"oauthAccount":{"accountUuid":"uuid-old"},"projects":{"/workspace":{}}}`,
	)
	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		ActiveAccountID: "old",
		Accounts: []agentauth.AccountRecord{
			{ID: "old", Label: "Old", Credential: oldCredential},
			{ID: "new", Label: "New", Credential: newCredential},
		},
	}}
	auth := newTestAuth(t, store)
	auth.credentials.validate = func(ctx context.Context, credential json.RawMessage) (agentauth.ValidatedAccount, error) {
		validated, err := acceptCredential(ctx, credential)
		validated.PlanType = "max"
		return validated, err
	}
	if err := auth.accounts.ActivateAccount(context.Background(), "new"); err != nil {
		t.Fatal(err)
	}
	credentials := readFile(t, filepath.Join(host, ".credentials.json"))
	config := readFile(t, filepath.Join(host, ".claude.json"))
	if !strings.Contains(credentials, `"new"`) || !strings.Contains(credentials, "kept") {
		t.Fatalf("host credentials = %s", credentials)
	}
	if !strings.Contains(config, "uuid-new") || !strings.Contains(config, "/workspace") {
		t.Fatalf("host config = %s", config)
	}
	snapshot := auth.accounts.AccountsSnapshot()
	if snapshot.ActiveAccountID != "new" || !snapshot.Items[1].Active || snapshot.Items[1].PlanType != "max" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "refreshToken") {
		t.Fatalf("credential leaked through account snapshot: %s", encoded)
	}
}

func TestActivateAccountFailureAndRunLeasePreserveCurrentCredential(t *testing.T) {
	host := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", host)
	writeHostFiles(t, host, `{"claudeAiOauth":{"refreshToken":"old"}}`, `{"oauthAccount":{"accountUuid":"uuid-old"}}`)
	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		ActiveAccountID: "old",
		Accounts: []agentauth.AccountRecord{
			{ID: "old", Label: "Old", Credential: testClaudeCredential("old", "uuid-old", "")},
			{ID: "new", Label: "New", Credential: testClaudeCredential("new", "uuid-new", "")},
		},
	}}
	auth := newTestAuth(t, store)
	release, err := auth.accounts.BeginRun()
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.accounts.ActivateAccount(context.Background(), "new"); !errors.Is(err, agentauth.ErrAccountInUse) {
		t.Fatalf("activate during run error = %v", err)
	}
	release()
	auth.credentials.validate = func(context.Context, json.RawMessage) (agentauth.ValidatedAccount, error) {
		return agentauth.ValidatedAccount{}, errors.New("expired credential")
	}
	if err := auth.accounts.ActivateAccount(context.Background(), "new"); err == nil || err.Error() != "expired credential" {
		t.Fatalf("invalid activation error = %v", err)
	}
	if got := readFile(t, filepath.Join(host, ".credentials.json")); !strings.Contains(got, `"old"`) || auth.accounts.AccountsSnapshot().ActiveAccountID != "old" {
		t.Fatalf("failed switch changed active account: credential=%s snapshot=%#v", got, auth.accounts.AccountsSnapshot())
	}
}

// The CLI refreshes the active account's tokens on the host. Switching away
// must keep them, or switching back would restore tokens the CLI has since
// rotated.
func TestActivateAccountKeepsOutgoingAccountsRefreshedLogin(t *testing.T) {
	host := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", host)
	writeHostFiles(t, host,
		`{"claudeAiOauth":{"refreshToken":"refreshed-one"}}`,
		`{"oauthAccount":{"accountUuid":"uuid-1","emailAddress":"one@example.test"}}`,
	)
	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		ActiveAccountID: "one",
		Accounts: []agentauth.AccountRecord{
			{ID: "one", Label: "One", Credential: testClaudeCredential("saved-one", "uuid-1", "one@example.test")},
			{ID: "two", Label: "Two", Credential: testClaudeCredential("saved-two", "uuid-2", "two@example.test")},
		},
	}}
	auth := newTestAuth(t, store)

	if err := auth.accounts.ActivateAccount(context.Background(), "two"); err != nil {
		t.Fatal(err)
	}
	one, _ := store.accounts.Find("one")
	if !strings.Contains(string(one.Credential), "refreshed-one") {
		t.Fatalf("outgoing account credential = %s", one.Credential)
	}
	credentials := readFile(t, filepath.Join(host, ".credentials.json"))
	config := readFile(t, filepath.Join(host, ".claude.json"))
	if !strings.Contains(credentials, "saved-two") || !strings.Contains(config, "uuid-2") || store.accounts.ActiveAccountID != "two" {
		t.Fatalf("after switch: credentials = %s, config = %s, active = %q", credentials, config, store.accounts.ActiveAccountID)
	}

	if err := auth.accounts.ActivateAccount(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if credentials := readFile(t, filepath.Join(host, ".credentials.json")); !strings.Contains(credentials, "refreshed-one") {
		t.Fatalf("switching back restored stale tokens: %s", credentials)
	}
}

// A host login for the active account may be newer than its saved copy, so
// startup keeps it. It reaches the vault only once a capture validates it.
func TestNewAuthKeepsNewerHostTokensForTheSameActiveAccount(t *testing.T) {
	host := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", host)
	writeHostFiles(t, host, `{"claudeAiOauth":{"refreshToken":"refreshed"}}`, `{"oauthAccount":{"accountUuid":"uuid-1"}}`)
	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		ActiveAccountID: "one",
		Accounts:        []agentauth.AccountRecord{{ID: "one", Label: "One", Credential: testClaudeCredential("stale", "uuid-1", "")}},
	}}
	auth := newTestAuth(t, store)
	if got := readFile(t, filepath.Join(host, ".credentials.json")); !strings.Contains(got, "refreshed") {
		t.Fatalf("host credential was overwritten: %s", got)
	}
	if saved := string(store.accounts.Accounts[0].Credential); !strings.Contains(saved, "stale") {
		t.Fatalf("startup saved an unvalidated host login: %s", saved)
	}

	if err := auth.accounts.CaptureAfterRun(context.Background()); err != nil {
		t.Fatal(err)
	}
	if saved := string(store.accounts.Accounts[0].Credential); !strings.Contains(saved, "refreshed") {
		t.Fatalf("validated host login was not saved: %s", saved)
	}
}

func TestNewAuthRestoresActiveAccountOverDifferentHostLogin(t *testing.T) {
	host := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", host)
	writeHostFiles(t, host, `{"claudeAiOauth":{"refreshToken":"other"}}`, `{"oauthAccount":{"accountUuid":"uuid-other"}}`)
	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		ActiveAccountID: "one",
		Accounts:        []agentauth.AccountRecord{{ID: "one", Label: "One", Credential: testClaudeCredential("saved", "uuid-1", "")}},
	}}
	if _, err := NewAuth(agentauth.NewAccountVault(store)); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(host, ".credentials.json")); !strings.Contains(got, "saved") {
		t.Fatalf("active account was not restored: %s", got)
	}
}

type memoryAccountStore struct{ accounts agentauth.AccountSet }

func (s *memoryAccountStore) AgentAccounts(context.Context, agent.ProviderID) (agentauth.AccountSet, error) {
	return s.accounts.Clone(), nil
}

func (s *memoryAccountStore) SaveAgentAccounts(_ context.Context, _ agent.ProviderID, accounts agentauth.AccountSet) error {
	s.accounts = accounts.Clone()
	return nil
}

// newTestAuth opens saved accounts over store with validation that accepts
// every credential.
func newTestAuth(t *testing.T, store agentauth.AccountStore) *Auth {
	t.Helper()
	auth, err := NewAuth(agentauth.NewAccountVault(store))
	if err != nil {
		t.Fatal(err)
	}
	auth.credentials.validate = acceptCredential
	return auth
}

func acceptCredential(_ context.Context, credential json.RawMessage) (agentauth.ValidatedAccount, error) {
	var stored accountCredential
	if err := json.Unmarshal(credential, &stored); err != nil {
		return agentauth.ValidatedAccount{}, err
	}
	return agentauth.ValidatedAccount{
		Email: oauthAccountEmail(stored.OAuthAccount), PlanType: "pro",
		Credential: append(json.RawMessage(nil), credential...),
	}, nil
}

// failCredentialWrites makes CLI file writes fail with "disk full" whenever
// fail returns true for the path.
func failCredentialWrites(t *testing.T, fail func(path string) bool) {
	t.Helper()
	t.Cleanup(func() { writeCredentialFile = agentauth.WriteCredentialFile })
	writeCredentialFile = func(path string, data []byte) error {
		if fail(path) {
			return fmt.Errorf("write %s: disk full", filepath.Base(path))
		}
		return agentauth.WriteCredentialFile(path, data)
	}
}

func testClaudeCredential(refreshToken, accountUUID, email string) json.RawMessage {
	credential, err := json.Marshal(accountCredential{
		Credentials: map[string]json.RawMessage{
			"claudeAiOauth": json.RawMessage(`{"refreshToken":"` + refreshToken + `"}`),
		},
		OAuthAccount: json.RawMessage(`{"accountUuid":"` + accountUUID + `","emailAddress":"` + email + `"}`),
	})
	if err != nil {
		panic(err)
	}
	return credential
}

func installFakeClaude(t *testing.T, body string) {
	t.Helper()
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeHostFiles(t *testing.T, dir, credentials, config string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(credentials), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
