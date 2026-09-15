package codex

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestParserReadsBothRateLimitWindows(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	events, err := parser.ParseLine([]byte(`{"type":"token_count","rate_limits":{` +
		`"primary_window":{"used_percent":12.5,"window_minutes":300,"resets_in_seconds":600},` +
		`"secondary_window":{"used_percent":40,"window_minutes":10080,"resets_in_seconds":3600}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected both windows, got %d events", len(events))
	}
	for _, ev := range events {
		if ev.Type != agent.EventQuotaUpdated || ev.Quota == nil {
			t.Fatalf("unexpected event: %#v", ev)
		}
		if ev.Quota.ResetsAt == 0 {
			t.Fatalf("%s window has no reset time: %#v", ev.Quota.Window, ev.Quota)
		}
	}
	if events[0].Quota.Window != agent.QuotaWindowSession || *events[0].Quota.UsedPercent != 12.5 {
		t.Fatalf("primary is not the session window: %#v", events[0].Quota)
	}
	if events[1].Quota.Window != agent.QuotaWindowWeekly || *events[1].Quota.UsedPercent != 40 {
		t.Fatalf("secondary is not the weekly window: %#v", events[1].Quota)
	}
}

// A run that reports only the short window must not leave a weekly reading
// behind, since an absent window and a window at 0% mean different things.
func TestParserReportsOnlyTheWindowsPresent(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	events, err := parser.ParseLine([]byte(
		`{"type":"token_count","rate_limits":{"primary_window":{"used_percent":5,"window_minutes":300}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Quota.Window != agent.QuotaWindowSession {
		t.Fatalf("expected the session window alone, got %#v", events)
	}
	// No reset time was offered, so none is invented.
	if events[0].Quota.ResetsAt != 0 {
		t.Fatalf("invented a reset time: %#v", events[0].Quota)
	}
}

// token_count carries usage counters too and arrives on every turn; a line
// with no rate_limits block must stay silent rather than file an empty reading.
func TestParserIgnoresTokenCountWithoutRateLimits(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	events, err := parser.ParseLine([]byte(`{"type":"token_count","info":{"total_token_usage":{"input_tokens":10}}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.Type == agent.EventQuotaUpdated {
			t.Fatalf("filed a quota reading from a line that had none: %#v", ev)
		}
	}
}
