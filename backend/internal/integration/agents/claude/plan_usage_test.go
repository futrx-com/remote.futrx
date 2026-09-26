package claude

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

// fakeClaudeUsage answers get_usage for whichever account the config
// directory signs in as, the way Claude Code's /usage data would.
const fakeClaudeUsage = `
case "$1" in
auth)
  printf '%s\n' '{"loggedIn":true,"authMethod":"claude.ai","subscriptionType":"max"}' ;;
-p)
  [ -z "$ANTHROPIC_API_KEY" ] || exit 4
  IFS= read -r request
  case "$request" in *'"subtype":"get_usage"'*'"skip_behaviors":true'*|*'"skip_behaviors":true'*'"subtype":"get_usage"'*) ;; *) exit 3 ;; esac
  config="$CLAUDE_CONFIG_DIR/.claude.json"
  account=$(sed -n 's/.*"emailAddress": *"\([^"]*\)".*/\1/p' "$config")
  case "$account" in
  one@example.test)
    printf '%s' '{"claudeAiOauth":{"refreshToken":"refreshed-one"}}' > "$CLAUDE_CONFIG_DIR/.credentials.json"
    printf '%s\n' '{"type":"control_response","response":{"subtype":"success","request_id":"remote-plan-usage","response":{"subscription_type":"max","rate_limits_available":true,"rate_limits":{"five_hour":{"utilization":24,"resets_at":"2026-09-25T18:00:00Z"},"seven_day":{"utilization":61.5,"resets_at":"2026-09-30T09:00:00.5+00:00"},"seven_day_sonnet":{"utilization":3,"resets_at":null}}}}}' ;;
  two@example.test)
    printf '%s\n' '{"type":"control_response","response":{"subtype":"error","request_id":"remote-plan-usage","error":"OAuth token has expired"}}' ;;
  three@example.test)
    printf '%s\n' '{"type":"control_response","response":{"subtype":"success","request_id":"remote-plan-usage","response":{"subscription_type":null,"rate_limits_available":false,"rate_limits":null}}}' ;;
  host@example.test)
    printf '%s\n' '{"type":"system","subtype":"status"}'
    printf '%s\n' '{"type":"control_response","response":{"subtype":"success","request_id":"remote-plan-usage","response":{"rate_limits_available":true,"rate_limits":{"five_hour":{"utilization":7,"resets_at":null},"seven_day":null}}}}' ;;
  esac ;;
esac
`

func usageWindows(windows []agent.Quota) []string {
	var out []string
	for _, window := range windows {
		entry := string(window.Window) + "="
		if window.UsedPercent != nil {
			entry += strconv.FormatFloat(*window.UsedPercent, 'f', -1, 64)
		}
		if window.ResetsAt != 0 {
			entry += "@" + time.Unix(window.ResetsAt, 0).UTC().Format(time.RFC3339)
		}
		out = append(out, entry)
	}
	return out
}

func TestReadPlanUsageReadsEverySavedAccountInItsOwnHome(t *testing.T) {
	installFakeClaude(t, fakeClaudeUsage)
	host := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", host)
	writeHostFiles(t, host,
		`{"claudeAiOauth":{"refreshToken":"saved-one"}}`,
		`{"oauthAccount":{"accountUuid":"uuid-one","emailAddress":"one@example.test"}}`,
	)
	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		ActiveAccountID: "one",
		Accounts: []agentauth.AccountRecord{
			{ID: "one", Label: "One", Credential: testClaudeCredential("saved-one", "uuid-one", "one@example.test")},
			{ID: "two", Label: "Two", Credential: testClaudeCredential("saved-two", "uuid-two", "two@example.test")},
			{ID: "three", Label: "Three", Credential: testClaudeCredential("saved-three", "uuid-three", "three@example.test")},
		},
	}}
	provider := newTestRuntime(agentmodule.BuildDependencies{
		Accounts: agentauth.NewAccountVault(store),
	}).Lookup(agent.ProviderClaude).(*Provider)

	usages := provider.ReadPlanUsage(context.Background())
	if len(usages) != 3 {
		t.Fatalf("usages = %#v; want one per saved account and none for the host", usages)
	}
	if usages[0].AccountID != "one" || usages[0].Err != nil ||
		!reflect.DeepEqual(usageWindows(usages[0].Windows), []string{
			"session=24@2026-09-25T18:00:00Z", "weekly=61.5@2026-09-30T09:00:00Z",
		}) {
		t.Fatalf("account one = %+v (%v)", usages[0], usageWindows(usages[0].Windows))
	}
	if usages[1].AccountID != "two" || usages[1].Err == nil || !strings.Contains(usages[1].Err.Error(), "OAuth token has expired") {
		t.Fatalf("account two = %+v", usages[1])
	}
	if usages[2].AccountID != "three" || usages[2].Err == nil || !strings.Contains(usages[2].Err.Error(), "did not report plan limits") {
		t.Fatalf("account three = %+v", usages[2])
	}

	// The CLI refreshed account one's sign-in while reading; the vault keeps
	// it, and nothing else changed.
	if saved := string(store.accounts.Accounts[0].Credential); !strings.Contains(saved, "refreshed-one") {
		t.Fatalf("refreshed login was not saved: %s", saved)
	}
	if saved := string(store.accounts.Accounts[1].Credential); !strings.Contains(saved, "saved-two") {
		t.Fatalf("account two changed: %s", saved)
	}
	if credentials := readFile(t, filepath.Join(host, ".credentials.json")); !strings.Contains(credentials, "saved-one") {
		t.Fatalf("the host login changed: %s", credentials)
	}
}

func TestReadPlanUsageReadsTheHostLoginOnlyWhileNoAccountIsActive(t *testing.T) {
	installFakeClaude(t, fakeClaudeUsage)
	host := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", host)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-api-key")
	writeHostFiles(t, host,
		`{"claudeAiOauth":{"refreshToken":"host"}}`,
		`{"oauthAccount":{"accountUuid":"uuid-host","emailAddress":"host@example.test"}}`,
	)

	for _, test := range []struct {
		name     string
		accounts agentauth.AccountSet
		want     []string
	}{
		{name: "no saved accounts", want: []string{"=session=7"}},
		{name: "saved account not active", accounts: agentauth.AccountSet{Accounts: []agentauth.AccountRecord{
			{ID: "three", Label: "Three", Credential: testClaudeCredential("saved-three", "uuid-three", "three@example.test")},
		}}, want: []string{"three=error", "=session=7"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &memoryAccountStore{accounts: test.accounts}
			provider := newTestRuntime(agentmodule.BuildDependencies{
				Accounts: agentauth.NewAccountVault(store),
			}).Lookup(agent.ProviderClaude).(*Provider)
			var got []string
			for _, usage := range provider.ReadPlanUsage(context.Background()) {
				entry := usage.AccountID + "="
				if usage.Err != nil {
					entry += "error"
				}
				entry += strings.Join(usageWindows(usage.Windows), ",")
				got = append(got, entry)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("usages = %v; want %v", got, test.want)
			}
		})
	}
}

func TestReadPlanUsageSkipsAHostWithoutASubscriptionLogin(t *testing.T) {
	installFakeClaude(t, `exit 9`)
	host := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", host)
	writeHostFiles(t, host, `{"mcpOAuth":{}}`, `{}`)
	provider := newTestRuntime(agentmodule.BuildDependencies{}).Lookup(agent.ProviderClaude).(*Provider)
	if usages := provider.ReadPlanUsage(context.Background()); len(usages) != 0 {
		t.Fatalf("usages = %+v", usages)
	}
}

func TestClaudeUsageReportKeepsOnlyReportedWindows(t *testing.T) {
	now := time.Unix(1_790_000_000, 0)
	for _, test := range []struct {
		name string
		raw  string
		want []string
	}{
		{name: "unavailable", raw: `{"rate_limits_available":false,"rate_limits":{"five_hour":{"utilization":5}}}`},
		{name: "null limits", raw: `{"rate_limits_available":true,"rate_limits":null}`},
		{name: "unknown utilization", raw: `{"rate_limits_available":true,"rate_limits":{"five_hour":{"utilization":null,"resets_at":"2026-09-25T18:00:00Z"}}}`},
		{name: "negative utilization", raw: `{"rate_limits_available":true,"rate_limits":{"seven_day":{"utilization":-1}}}`},
		{name: "unreadable reset", raw: `{"rate_limits_available":true,"rate_limits":{"seven_day":{"utilization":100,"resets_at":"soon"}}}`, want: []string{"weekly=100"}},
		{name: "zero", raw: `{"rate_limits_available":true,"rate_limits":{"five_hour":{"utilization":0,"resets_at":"2026-09-25T18:00:00Z"}}}`, want: []string{"session=0@2026-09-25T18:00:00Z"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var report claudeUsageReport
			if err := json.Unmarshal([]byte(test.raw), &report); err != nil {
				t.Fatal(err)
			}
			windows := report.windows(now)
			if got := usageWindows(windows); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("windows = %v; want %v", got, test.want)
			}
			for _, window := range windows {
				if window.MeasuredAt != now.UnixMilli() {
					t.Fatalf("measured at %d; want %d", window.MeasuredAt, now.UnixMilli())
				}
			}
		})
	}
}
