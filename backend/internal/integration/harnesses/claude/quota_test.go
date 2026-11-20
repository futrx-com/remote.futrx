package claude

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// TestTheFiveHourWindowIsRead uses the exact line the CLI emitted on this
// platform, so the test fails if the shape ever moves.
func TestTheFiveHourWindowIsRead(t *testing.T) {
	line := `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":1787563200,"rateLimitType":"five_hour","overageStatus":"rejected","overageDisabledReason":"org_level_disabled","isUsingOverage":false},"uuid":"4d7f37b1","session_id":"92387bf9"}`

	events, err := NewParser(agent.RunRequest{}).ParseLine([]byte(line))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	quota := onlyQuota(t, events)

	if quota.Window != agent.QuotaWindowSession {
		t.Errorf("window = %q, want the session window", quota.Window)
	}
	if quota.ResetsAt != 1787563200 {
		t.Errorf("resetsAt = %d, want the CLI's own timestamp", quota.ResetsAt)
	}
	if quota.Status != "allowed" {
		t.Errorf("status = %q, want the CLI's own word", quota.Status)
	}
	// Claude reports a status, not a number. Storing an absent percentage as
	// zero would draw an empty gauge and read as "none of your plan is used".
	if quota.UsedPercent != nil {
		t.Errorf("usedPercent = %v, want it absent when the CLI sends none", *quota.UsedPercent)
	}
	if quota.MeasuredAt == 0 {
		t.Error("a reading with no measured-at cannot be shown as a snapshot")
	}
}

func TestTheWeeklyWindowIsRead(t *testing.T) {
	line := `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed_warning","resetsAt":1788000000,"rateLimitType":"seven_day","utilization":0.82}}`

	events, err := NewParser(agent.RunRequest{}).ParseLine([]byte(line))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	quota := onlyQuota(t, events)

	if quota.Window != agent.QuotaWindowWeekly {
		t.Errorf("window = %q, want the weekly window", quota.Window)
	}
	if quota.UsedPercent == nil || *quota.UsedPercent != 82 {
		t.Errorf("usedPercent = %v, want a 0–1 utilization scaled to a percentage", quota.UsedPercent)
	}
	if quota.Status != "allowed_warning" {
		t.Errorf("status = %q", quota.Status)
	}
}

// TestAnUnknownWindowIsDropped is the important one. A vendor adding a third
// window must not have it filed under one of the two the card draws — a
// confidently wrong gauge is worse than a missing one.
func TestAnUnknownWindowIsDropped(t *testing.T) {
	line := `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","rateLimitType":"thirty_day"}}`

	events, err := NewParser(agent.RunRequest{}).ParseLine([]byte(line))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	for _, ev := range events {
		if ev.Type == agent.EventQuotaUpdated {
			t.Fatalf("an unrecognized window was stored as %+v", ev.Quota)
		}
	}
}

// TestAMalformedPayloadIsIgnored: a quota line is a courtesy, and a turn must
// not fail because one of them could not be parsed.
func TestAMalformedPayloadIsIgnored(t *testing.T) {
	for _, line := range []string{
		`{"type":"rate_limit_event"}`,
		`{"type":"rate_limit_event","rate_limit_info":"not an object"}`,
		`{"type":"rate_limit_event","rate_limit_info":{}}`,
	} {
		events, err := NewParser(agent.RunRequest{}).ParseLine([]byte(line))
		if err != nil {
			t.Fatalf("Parse(%s) = %v, want the line ignored", line, err)
		}
		for _, ev := range events {
			if ev.Type == agent.EventQuotaUpdated {
				t.Errorf("Parse(%s) produced a reading: %+v", line, ev.Quota)
			}
		}
	}
}

func onlyQuota(t *testing.T, events []agent.Event) agent.Quota {
	t.Helper()
	var found *agent.Quota
	for _, ev := range events {
		if ev.Type != agent.EventQuotaUpdated {
			continue
		}
		if found != nil {
			t.Fatal("one line produced more than one reading")
		}
		if ev.Quota == nil {
			t.Fatal("a quota event with no quota on it")
		}
		found = ev.Quota
	}
	if found == nil {
		t.Fatal("no quota reading was produced")
	}
	return *found
}
