package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

// installFakeCodexUsage puts a codex command first on PATH whose app server
// answers account/rateLimits/read for the account in its CODEX_HOME, the way
// Codex's /status data would.
func installFakeCodexUsage(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := `#!/bin/sh
[ "$1" = app-server ] || exit 2
[ -z "$OPENAI_API_KEY" ] || exit 4
account=$(sed -n 's/.*"account_id": *"\([^"]*\)".*/\1/p' "$CODEX_HOME/auth.json")
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"method":"account/rateLimits/read"'*)
      case "$line" in *'"excludeResetCreditDetails":true'*) ;; *) exit 3 ;; esac
      case "$account" in
        acct-personal)
          printf '%s' '{"auth_mode":"chatgpt","tokens":{"account_id":"acct-personal","access_token":"refreshed"}}' > "$CODEX_HOME/auth.json"
          printf '%s\n' '{"id":2,"result":{"rateLimits":{"limitId":"codex","primary":{"usedPercent":45,"windowDurationMins":300,"resetsAt":1790000000},"secondary":{"usedPercent":8,"windowDurationMins":10080,"resetsAt":1790500000}},"rateLimitsByLimitId":{"codex":{"limitId":"codex","primary":{"usedPercent":45,"windowDurationMins":300,"resetsAt":1790000000},"secondary":{"usedPercent":8,"windowDurationMins":10080,"resetsAt":1790500000}},"codex_other":{"limitId":"codex_other","primary":{"usedPercent":99,"windowDurationMins":300}}}}}' ;;
        acct-work) printf '%s\n' '{"id":2,"error":{"code":-32000,"message":"usage is unavailable for this workspace"}}' ;;
        acct-host) printf '%s\n' '{"id":2,"result":{"rateLimits":{"limitId":"codex","primary":{"usedPercent":10,"windowDurationMins":10080}}}}' ;;
      esac ;;
  esac
done
`
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func planUsageSummary(usages []agent.AccountPlanUsage) []string {
	var out []string
	for _, usage := range usages {
		entry := usage.AccountID + ":"
		if usage.Err != nil {
			entry += " error=" + usage.Err.Error()
		}
		for _, window := range usage.Windows {
			entry += " " + string(window.Window) + "=" + strconv.FormatFloat(*window.UsedPercent, 'f', -1, 64)
			if window.ResetsAt != 0 {
				entry += "@" + strconv.FormatInt(window.ResetsAt, 10)
			}
		}
		out = append(out, entry)
	}
	return out
}

func TestReadPlanUsageReadsEverySavedAccountInItsOwnHome(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("OPENAI_API_KEY", "sk-test")
	installFakeCodexUsage(t)
	personal := codexTestCredential("personal", "saved")
	writeCodexHostCredential(t, filepath.Join(codexHome, "auth.json"), personal)
	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		ActiveAccountID: "personal",
		Accounts: []agentauth.AccountRecord{
			{ID: "personal", Label: "Personal", Credential: personal},
			{ID: "work", Label: "Work", Credential: codexTestCredential("work", "saved")},
		},
	}}
	auth := newTestAuth(t, store)
	auth.credentials.validate = echoCodexValidator("personal@example.test")
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	provider.accounts = auth.accounts

	got := planUsageSummary(provider.ReadPlanUsage(context.Background()))
	want := []string{
		"personal: session=45@1790000000 weekly=8@1790500000",
		"work: error=Codex could not report usage: usage is unavailable for this workspace",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("usages = %q; want %q", got, want)
	}

	// The app server refreshed the personal login while reading; the vault
	// keeps it, and the host login and the other account are untouched.
	accounts := store.saved()
	if saved := string(accounts.Accounts[0].Credential); !strings.Contains(saved, `"refreshed"`) {
		t.Fatalf("refreshed login was not saved: %s", saved)
	}
	requireCodexHostCredential(t, filepath.Join(codexHome, "auth.json"), personal)
	if saved := string(accounts.Accounts[1].Credential); saved != string(codexTestCredential("work", "saved")) {
		t.Fatalf("work account changed: %s", saved)
	}
}

func TestReadPlanUsageReadsTheHostLoginOnlyWhileNoAccountIsActive(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	installFakeCodexUsage(t)
	writeCodexHostCredential(t, filepath.Join(codexHome, "auth.json"), codexTestCredential("host", "token"))

	withoutAccounts := newTestProvider(nil, provisioning.ContainerDependencies{})
	if got, want := planUsageSummary(withoutAccounts.ReadPlanUsage(context.Background())), []string{": weekly=10"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("host usage = %q; want %q", got, want)
	}

	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		Accounts: []agentauth.AccountRecord{{ID: "work", Label: "Work", Credential: codexTestCredential("work", "saved")}},
	}}
	auth := newTestAuth(t, store)
	auth.credentials.validate = echoCodexValidator("work@example.test")
	inactive := newTestProvider(nil, provisioning.ContainerDependencies{})
	inactive.accounts = auth.accounts
	got := planUsageSummary(inactive.ReadPlanUsage(context.Background()))
	if len(got) != 2 || !strings.HasPrefix(got[0], "work: error=") || got[1] != ": weekly=10" {
		t.Fatalf("usages with no active account = %q", got)
	}
}

func TestReadPlanUsageSkipsAHostWithoutASubscriptionLogin(t *testing.T) {
	for name, credential := range map[string]string{
		"signed out": "",
		"API key":    `{"auth_mode":"apikey","OPENAI_API_KEY":"sk-test"}`,
	} {
		t.Run(name, func(t *testing.T) {
			codexHome := t.TempDir()
			t.Setenv("CODEX_HOME", codexHome)
			installFakeCodexUsage(t)
			if credential != "" {
				writeCodexHostCredential(t, filepath.Join(codexHome, "auth.json"), []byte(credential))
			}
			if usages := newTestProvider(nil, provisioning.ContainerDependencies{}).ReadPlanUsage(context.Background()); len(usages) != 0 {
				t.Fatalf("usages = %+v", usages)
			}
		})
	}
}

func TestReadCodexRateLimitsReportsAnAppServerThatNeverAnswers(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := readCodexRateLimits(ctx, t.TempDir()); err == nil ||
		(!strings.Contains(err.Error(), "closed before reporting usage") && !errors.Is(err, syscall.EPIPE)) {
		t.Fatalf("error = %v", err)
	}
}
