package codexharness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestCodexQuotaReadingsSelectProductAndWindowDuration(t *testing.T) {
	const now = int64(1787563000123)
	for _, test := range []struct {
		name string
		raw  string
		want []agent.Quota
	}{
		{
			name: "absolute reset and duration determine windows",
			raw:  `{"rateLimits":{"limitId":"codex","primary":{"usedPercent":80,"windowDurationMins":10080,"resetsAt":1788000000},"secondary":{"usedPercent":0,"windowDurationMins":300,"resetsAt":1787563200}}}`,
			want: []agent.Quota{quotaReading(agent.QuotaWindowSession, 0, 1787563200, now), quotaReading(agent.QuotaWindowWeekly, 80, 1788000000, now)},
		},
		{
			name: "weekly only primary",
			raw:  `{"rateLimits":{"limitId":"codex","primary":{"usedPercent":0,"windowDurationMins":10080,"resetsAt":1788000000}}}`,
			want: []agent.Quota{quotaReading(agent.QuotaWindowWeekly, 0, 1788000000, now)},
		},
		{
			name: "legacy bucket without identity",
			raw:  `{"rateLimits":{"primary":{"usedPercent":12.5,"windowDurationMins":300,"resetsAt":null}}}`,
			want: []agent.Quota{quotaReading(agent.QuotaWindowSession, 12.5, 0, now)},
		},
		{
			name: "codex map bucket overrides legacy and unrelated products",
			raw:  `{"rateLimits":{"limitId":"codex_other","primary":{"usedPercent":99,"windowDurationMins":300}},"rateLimitsByLimitId":{"codex":{"limitId":"codex","primary":{"usedPercent":10,"windowDurationMins":300}},"other":"invalid unrelated product"}}`,
			want: []agent.Quota{quotaReading(agent.QuotaWindowSession, 10, 0, now)},
		},
		{
			name: "duplicate duration emits only first reading",
			raw:  `{"rateLimits":{"primary":{"usedPercent":10,"windowDurationMins":300},"secondary":{"usedPercent":20,"windowDurationMins":300}}}`,
			want: []agent.Quota{quotaReading(agent.QuotaWindowSession, 10, 0, now)},
		},
		{name: "explicit other product", raw: `{"rateLimits":{"limitId":"codex_other","primary":{"usedPercent":20,"windowDurationMins":300}}}`},
		{name: "map without codex has no fallback", raw: `{"rateLimits":{"primary":{"usedPercent":20,"windowDurationMins":300}},"rateLimitsByLimitId":{"other":{}}}`},
		{name: "mismatched map identity", raw: `{"rateLimitsByLimitId":{"codex":{"limitId":"other","primary":{"usedPercent":20,"windowDurationMins":300}}}}`},
		{name: "unknown durations", raw: `{"rateLimits":{"primary":{"usedPercent":20,"windowDurationMins":15},"secondary":{"usedPercent":20,"windowDurationMins":43200}}}`},
		{name: "missing duration", raw: `{"rateLimits":{"primary":{"usedPercent":20}}}`},
		{name: "missing percentage", raw: `{"rateLimits":{"primary":{"windowDurationMins":300}}}`},
		{name: "negative percentage", raw: `{"rateLimits":{"primary":{"usedPercent":-1,"windowDurationMins":300}}}`},
		{name: "missing payload", raw: `{}`},
		{name: "null payload", raw: `{"rateLimits":null}`},
		{name: "malformed payload", raw: `{"rateLimits":"not an object"}`},
		{name: "wrong number type", raw: `{"rateLimits":{"primary":{"usedPercent":"20","windowDurationMins":300}}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := codexQuotaReadings(json.RawMessage(test.raw), now); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("readings = %#v; want %#v", got, test.want)
			}
		})
	}
}

func quotaReading(window agent.QuotaWindow, percent float64, reset, measured int64) agent.Quota {
	return agent.Quota{Window: window, UsedPercent: &percent, ResetsAt: reset, MeasuredAt: measured}
}

func TestAppServerQuotaNotificationPreservesNativeEventAndUsage(t *testing.T) {
	parser := newAppServerEventParser(agent.RunRequest{Provider: agent.ProviderCodex, ConversationID: "chat-1"}, "Codex")
	parser.ParseNotification("thread/tokenUsage/updated", json.RawMessage(`{"tokenUsage":{"last":{"inputTokens":10,"outputTokens":4}}}`))
	raw := json.RawMessage(`{"rateLimits":{"limitId":"codex","primary":{"usedPercent":25,"windowDurationMins":300,"resetsAt":1787563200}}}`)
	events := parser.ParseNotification("account/rateLimits/updated", raw)
	if len(events) != 2 || events[0].Type != agent.EventProviderNative || events[1].Type != agent.EventQuotaUpdated {
		t.Fatalf("events = %#v", events)
	}
	for _, event := range events {
		if event.Provider != agent.ProviderCodex || event.ConversationID != "chat-1" || event.Native == nil || event.Native.Method != "account/rateLimits/updated" || string(event.Raw) != string(raw) {
			t.Fatalf("event identity or native payload lost: %#v", event)
		}
	}
	if string(events[0].Data) != string(raw) || events[1].Quota == nil || events[1].Quota.MeasuredAt != events[1].T {
		t.Fatalf("native data or measured time lost: %#v", events)
	}
	completed := parser.ParseNotification("turn/completed", json.RawMessage(`{"turn":{"status":"completed"}}`))
	usage, ok := agent.ParseUsage(completed[0].Usage)
	if !ok || usage.InputTokens != 10 || usage.OutputTokens != 4 {
		t.Fatalf("quota replaced token usage: %#v", usage)
	}

	for _, provider := range []agent.ProviderID{agent.ProviderCodex, agent.ProviderMiniMax} {
		parser := newAppServerEventParser(agent.RunRequest{Provider: provider}, string(provider))
		payload := raw
		if provider == agent.ProviderCodex {
			payload = json.RawMessage(`{"rateLimits":"malformed"}`)
		}
		if events := parser.ParseNotification("account/rateLimits/updated", payload); len(events) != 1 || events[0].Type != agent.EventProviderNative {
			t.Fatalf("invalid or unrelated-provider quota changed native fallback: %#v", events)
		}
	}
}

// A turn reports its account's windows through the app server's own rate-limit
// updates. Remote never reads them: the app server finishes in-flight requests
// before it exits, so an unanswered read would hold back the end of the turn.
func TestRunAppServerCollectsLiveQuotaWithoutReadingIt(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "read-requested")
	script := fmt.Sprintf(`
while IFS= read -r line; do
 case "$line" in
  *'account/rateLimits/read'*) : > %q ;;
  *'"id":1'*) printf '%%s\n' '{"id":1,"result":{}}' ;;
  *'"id":2'*) printf '%%s\n' '{"id":2,"result":{"thread":{"id":"thread-1"},"model":"gpt-test"}}' ;;
  *'"id":3'*)
   printf '%%s\n' '{"id":3,"result":{"turn":{"id":"turn-1","status":"inProgress"}}}'
   printf '%%s\n' '{"method":"account/rateLimits/updated","params":{"rateLimits":{"limitId":"codex","primary":{"usedPercent":40,"windowDurationMins":300}}}}'
   printf '%%s\n' '{"method":"account/rateLimits/updated","params":{"rateLimits":{"limitId":"codex","secondary":{"usedPercent":80,"windowDurationMins":10080}}}}'
   printf '%%s\n' '{"method":"turn/completed","params":{"turn":{"status":"completed"}}}'
   ;;
 esac
done`, marker)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var events []agent.Event
	err := Run(ctx, exec.CommandContext(ctx, "sh", "-c", script), agent.RunRequest{Provider: agent.ProviderCodex, ConversationID: "chat-1", Prompt: "work"}, "Codex", func(event agent.Event) { events = append(events, event) })
	if err != nil {
		t.Fatal(err)
	}
	var windows []string
	for _, event := range events {
		if event.Type == agent.EventQuotaUpdated {
			windows = append(windows, fmt.Sprintf("%s=%v", event.Quota.Window, *event.Quota.UsedPercent))
		}
	}
	if want := []string{"session=40", "weekly=80"}; !reflect.DeepEqual(windows, want) || events[len(events)-1].Type != agent.EventRunCompleted {
		t.Fatalf("quota windows = %v; want %v; events = %#v", windows, want, events)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the harness requested account/rateLimits/read (marker stat: %v)", err)
	}
}
