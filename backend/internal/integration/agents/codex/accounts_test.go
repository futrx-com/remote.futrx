package codex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

func TestReadCodexAccountRefreshesAndReturnsPlanMetadata(t *testing.T) {
	stdout := strings.NewReader(
		`{"id":1,"result":{}}` + "\n" +
			`{"id":2,"result":{"account":{"type":"chatgpt","email":"person@example.test","planType":"pro"},"requiresOpenaiAuth":true}}` + "\n",
	)
	var stdin bytes.Buffer
	account, err := readCodexAccountRPC(&stdin, stdout)
	if err != nil {
		t.Fatal(err)
	}
	if account.Account == nil || account.Account.Email != "person@example.test" || account.Account.PlanType != "pro" {
		t.Fatalf("account = %#v", account)
	}
	if !strings.Contains(stdin.String(), `"method":"account/read"`) || !strings.Contains(stdin.String(), `"refreshToken":true`) {
		t.Fatalf("requests = %s", stdin.String())
	}
}

func TestSnapCredentialPathUsesSnapManagedCodexHome(t *testing.T) {
	got, ok := snapCredentialPathFor("/snap/bin/codex", "/home/person")
	if !ok || got != "/home/person/snap/codex/current/auth.json" {
		t.Fatalf("snap credential path = %q, %v", got, ok)
	}
	if _, ok := snapCredentialPathFor("/usr/local/bin/codex", "/home/person"); ok {
		t.Fatal("non-Snap Codex was detected as a Snap command")
	}
}

func TestCodexAccountIdentityReadsTheChatGPTAccount(t *testing.T) {
	claims := `{"email":" Person@Example.TEST ","email_verified":true,"sub":"auth0|person",` +
		`"https://api.openai.com/auth":{"chatgpt_account_id":"acct-claim","chatgpt_plan_type":"plus",` +
		`"chatgpt_user_id":"user-chatgpt","user_id":"user-legacy"}}`
	legacyClaims := `{"email":"person@example.test","https://api.openai.com/auth":{"chatgpt_account_id":"acct-claim","user_id":"user-legacy"}}`
	emailClaims := `{"email":" Person@Example.TEST ","https://api.openai.com/auth":{"chatgpt_account_id":"acct-claim"}}`
	idToken := func(claims string) string {
		return "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".signature"
	}
	padded := "eyJhbGciOiJSUzI1NiJ9." + base64.URLEncoding.EncodeToString([]byte(claims)) + ".signature"
	if !strings.Contains(padded, "=.") {
		t.Fatal("the padded token fixture must end its payload with padding")
	}
	// auth builds a Codex auth.json with the fields the real CLI writes.
	auth := func(tokens string) json.RawMessage {
		return json.RawMessage(`{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":` + tokens + `,"last_refresh":"2026-09-01T00:00:00Z"}`)
	}
	// The email can change, so it names the person only without a user ID.
	fromClaims := agentauth.AccountIdentity{"account": "acct-claim", "user": "user-chatgpt"}

	cases := []struct {
		name       string
		credential json.RawMessage
		want       agentauth.AccountIdentity
	}{
		{"id token claims", auth(`{"id_token":"` + idToken(claims) + `","access_token":"a","refresh_token":"r"}`), fromClaims},
		{"account_id before the claim", auth(`{"id_token":"` + idToken(claims) + `","account_id":"acct-token"}`),
			agentauth.AccountIdentity{"account": "acct-token", "user": "user-chatgpt"}},
		{"legacy user_id claim", auth(`{"id_token":"` + idToken(legacyClaims) + `"}`),
			agentauth.AccountIdentity{"account": "acct-claim", "user": "user-legacy"}},
		{"padded payload", auth(`{"id_token":"` + padded + `"}`), fromClaims},
		{"email without a user ID", auth(`{"id_token":"` + idToken(emailClaims) + `"}`),
			agentauth.AccountIdentity{"account": "acct-claim", "email": "person@example.test"}},
		{"malformed id token keeps account_id", auth(`{"id_token":"not-a-jwt","account_id":"acct-token"}`),
			agentauth.AccountIdentity{"account": "acct-token"}},
		{"undecodable id token", auth(`{"id_token":"e30.%%%.signature"}`), nil},
		{"id token that is not JSON", auth(`{"id_token":"` + idToken("not json") + `"}`), nil},
		{"no tokens", auth(`null`), nil},
		{"API key", json.RawMessage(`{"auth_mode":"apikey","OPENAI_API_KEY":"sk-test"}`), nil},
		{"API key with stale tokens", json.RawMessage(`{"auth_mode":"apikey","tokens":{"account_id":"acct-token"}}`), nil},
		{"not JSON", json.RawMessage(`not json`), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := codexAccountIdentity(tc.credential)
			if !maps.Equal(got, tc.want) {
				t.Fatalf("identity = %#v, want %#v", got, tc.want)
			}
			if got.Known() != (len(tc.want) > 0) {
				t.Fatalf("Known() = %v for %#v", got.Known(), got)
			}
		})
	}
}

func TestNewAuthReconcilesHostLoginWithActiveAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	hostPath := filepath.Join(home, "auth.json")
	saved := codexTestCredential("personal", "saved")
	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		ActiveAccountID: "personal",
		Accounts:        []agentauth.AccountRecord{{ID: "personal", Label: "Personal", Credential: saved}},
	}}

	// The CLI may have refreshed the active account's login since it was
	// saved, so a login for the same account stays.
	refreshed := codexTestCredential("personal", "refreshed")
	writeCodexHostCredential(t, hostPath, refreshed)
	newTestAuth(t, store)
	requireCodexHostCredential(t, hostPath, refreshed)

	writeCodexHostCredential(t, hostPath, codexTestCredential("other", "other"))
	newTestAuth(t, store)
	requireCodexHostCredential(t, hostPath, saved)

	if err := os.Remove(hostPath); err != nil {
		t.Fatal(err)
	}
	newTestAuth(t, store)
	requireCodexHostCredential(t, hostPath, saved)
}

func TestAccountLoginCapturesCredentialWrittenThroughHome(t *testing.T) {
	binDir := t.TempDir()
	script := filepath.Join(binDir, "codex")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
mkdir -p "$HOME/.codex"
printf '%s' '{"auth_mode":"chatgpt","tokens":{"account_id":"acct-new","access_token":"new"}}' > "$HOME/.codex/auth.json"
printf '%s\n' 'https://auth.openai.com/codex/device'
printf '%s\n' 'ABCD-12345'
`), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", filepath.Join(t.TempDir(), "host-home"))
	canonicalHome := filepath.Join(t.TempDir(), ".codex")
	t.Setenv("CODEX_HOME", canonicalHome)

	store := &memoryAccountStore{}
	auth := newTestAuth(t, store)
	auth.credentials.validate = echoCodexValidator("new@example.test")
	if _, err := auth.accounts.StartAccountLogin(context.Background(), "Company", ""); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for auth.LoginState().Active && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state := auth.LoginState()
	if !state.Completed || state.Error != "" {
		t.Fatalf("login state = %#v", state)
	}
	saved := store.saved()
	if len(saved.Accounts) != 1 || saved.ActiveAccountID == "" {
		t.Fatalf("saved accounts = %#v", saved)
	}
	requireCodexHostCredential(t, filepath.Join(canonicalHome, "auth.json"), saved.Accounts[0].Credential)
}

func TestAccountLoginFinishCapturesExternalCredentialPath(t *testing.T) {
	externalPath := filepath.Join(t.TempDir(), "snap", "codex", "current", "auth.json")
	login, err := newAccountLogin([]string{externalPath})
	if err != nil {
		t.Fatal(err)
	}
	// The CLI ignored the isolated CODEX_HOME and wrote the Snap location.
	written := codexTestCredential("company", "new")
	writeCodexHostCredential(t, externalPath, written)

	credential, err := login.Finish(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if string(credential) != string(written) {
		t.Fatalf("login credential = %s", credential)
	}
	if _, err := os.Stat(externalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external credential that did not exist before the login was kept: %v", err)
	}
	requireRemoved(t, login.root)
}

func TestAccountLoginFinishPrefersIsolatedCredentialAndRestoresHost(t *testing.T) {
	externalPath := filepath.Join(t.TempDir(), "auth.json")
	previous := codexTestCredential("personal", "old")
	writeCodexHostCredential(t, externalPath, previous)
	login, err := newAccountLogin([]string{externalPath})
	if err != nil {
		t.Fatal(err)
	}
	isolated := codexTestCredential("company", "isolated")
	writeCodexHostCredential(t, filepath.Join(login.home, "auth.json"), isolated)
	writeCodexHostCredential(t, externalPath, codexTestCredential("company", "external"))

	credential, err := login.Finish(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if string(credential) != string(isolated) {
		t.Fatalf("login credential = %s, want the isolated one", credential)
	}
	requireCodexHostCredential(t, externalPath, previous)
	requireRemoved(t, login.root)
}

func TestFailedAccountLoginRestoresPreviousHostCredential(t *testing.T) {
	externalPath := filepath.Join(t.TempDir(), "snap", "codex", "current", "auth.json")
	previous := codexTestCredential("personal", "old")
	writeCodexHostCredential(t, externalPath, previous)
	login, err := newAccountLogin([]string{externalPath})
	if err != nil {
		t.Fatal(err)
	}
	writeCodexHostCredential(t, externalPath, codexTestCredential("company", "new"))

	_, err = login.Finish(errors.New("exit status 1"), "Error logging in: access denied")
	if err == nil || err.Error() != "codex login failed: exit status 1; codex output: Error logging in: access denied" {
		t.Fatalf("finish error = %v", err)
	}
	requireCodexHostCredential(t, externalPath, previous)
	requireRemoved(t, login.root)
}

func TestAbortedAccountLoginRestoresPreviousHostCredential(t *testing.T) {
	externalPath := filepath.Join(t.TempDir(), "auth.json")
	previous := codexTestCredential("personal", "old")
	writeCodexHostCredential(t, externalPath, previous)
	login, err := newAccountLogin([]string{externalPath})
	if err != nil {
		t.Fatal(err)
	}
	writeCodexHostCredential(t, externalPath, codexTestCredential("company", "new"))

	login.Abort()
	requireCodexHostCredential(t, externalPath, previous)
	requireRemoved(t, login.root)
}

func TestActivateAccountValidatesBeforeReplacingCurrentCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	hostPath := filepath.Join(home, "auth.json")
	oldCredential := codexTestCredential("old", "old")
	newCredential := codexTestCredential("new", "new")
	writeCodexHostCredential(t, hostPath, oldCredential)
	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		ActiveAccountID: "old",
		Accounts: []agentauth.AccountRecord{
			{ID: "old", Label: "Old", Credential: oldCredential},
			{ID: "new", Label: "New", Credential: newCredential},
		},
	}}
	auth := newTestAuth(t, store)
	validator := echoCodexValidator("new@example.test")
	auth.credentials.validate = func(ctx context.Context, credential json.RawMessage) (agentauth.ValidatedAccount, error) {
		validated, err := validator(ctx, credential)
		validated.PlanType = "pro"
		return validated, err
	}
	if err := auth.accounts.ActivateAccount(context.Background(), "new"); err != nil {
		t.Fatal(err)
	}
	requireCodexHostCredential(t, hostPath, newCredential)
	snapshot := auth.accounts.AccountsSnapshot()
	if snapshot.ActiveAccountID != "new" || !snapshot.Items[1].Active || snapshot.Items[1].PlanType != "pro" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "access_token") {
		t.Fatalf("credential leaked through account snapshot: %s", encoded)
	}
}

func TestActivateAccountFailureAndRunLeasePreserveCurrentCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	hostPath := filepath.Join(home, "auth.json")
	oldCredential := codexTestCredential("old", "old")
	writeCodexHostCredential(t, hostPath, oldCredential)
	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		ActiveAccountID: "old",
		Accounts: []agentauth.AccountRecord{
			{ID: "old", Label: "Old", Credential: oldCredential},
			{ID: "new", Label: "New", Credential: codexTestCredential("new", "new")},
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
	requireCodexHostCredential(t, hostPath, oldCredential)
	if active := auth.accounts.AccountsSnapshot().ActiveAccountID; active != "old" {
		t.Fatalf("failed switch changed the active account to %q", active)
	}
}

func TestImportCurrentAccountLabelsAreBoundedAndUnique(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	store := &memoryAccountStore{}
	auth := newTestAuth(t, store)
	auth.credentials.validate = echoCodexValidator("personal@example.test")
	if err := auth.accounts.ImportCurrent(context.Background(), "Personal"); err == nil || err.Error() != "Codex is not signed in" {
		t.Fatalf("import while signed out error = %v", err)
	}

	writeCodexHostCredential(t, filepath.Join(home, "auth.json"), codexTestCredential("personal", "token"))
	if err := auth.accounts.ImportCurrent(context.Background(), "  Personal  "); err != nil {
		t.Fatal(err)
	}
	for label, want := range map[string]error{
		"personal":              agentauth.ErrAccountLabelConflict,
		strings.Repeat("x", 65): agentauth.ErrAccountLabelInvalid,
		"   ":                   agentauth.ErrAccountLabelRequired,
	} {
		if err := auth.accounts.ImportCurrent(context.Background(), label); !errors.Is(err, want) {
			t.Errorf("import %q error = %v, want %v", label, err, want)
		}
	}
	snapshot := auth.accounts.AccountsSnapshot()
	if len(snapshot.Items) != 1 || snapshot.Items[0].Label != "Personal" || !snapshot.Items[0].Active {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "access_token") {
		t.Fatalf("credential leaked through account snapshot: %s", encoded)
	}
}

// codexTestCredential returns a ChatGPT auth.json for the ChatGPT account
// acct-<account>; token tells refreshed copies of one login apart.
func codexTestCredential(account, token string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"auth_mode":"chatgpt","tokens":{"account_id":"acct-%s","access_token":%q}}`, account, token))
}

// echoCodexValidator accepts every credential as the account email without
// refreshing it.
func echoCodexValidator(email string) func(context.Context, json.RawMessage) (agentauth.ValidatedAccount, error) {
	return func(_ context.Context, credential json.RawMessage) (agentauth.ValidatedAccount, error) {
		return agentauth.ValidatedAccount{
			Email: email, PlanType: "plus",
			Credential: append(json.RawMessage(nil), credential...),
		}, nil
	}
}

func newTestAuth(t *testing.T, store agentauth.AccountStore) *Auth {
	t.Helper()
	auth, err := NewAuth(agentauth.NewAccountVault(store))
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func writeCodexHostCredential(t *testing.T, path string, credential json.RawMessage) {
	t.Helper()
	if err := agentauth.WriteCredentialFile(path, credential); err != nil {
		t.Fatal(err)
	}
}

func requireCodexHostCredential(t *testing.T, path string, want json.RawMessage) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("credential at %s = %s, want %s", path, got, want)
	}
}

func requireRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s was left behind: %v", path, err)
	}
}

// memoryAccountStore is safe for the login goroutine that saves a finished
// account login while a test reads the saved set.
type memoryAccountStore struct {
	mu       sync.Mutex
	accounts agentauth.AccountSet
}

func (s *memoryAccountStore) AgentAccounts(context.Context, agent.ProviderID) (agentauth.AccountSet, error) {
	return s.saved(), nil
}

func (s *memoryAccountStore) SaveAgentAccounts(_ context.Context, _ agent.ProviderID, accounts agentauth.AccountSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts = accounts.Clone()
	return nil
}

func (s *memoryAccountStore) saved() agentauth.AccountSet {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accounts.Clone()
}
