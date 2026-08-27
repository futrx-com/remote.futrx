package service

import (
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
)

func TestNotificationKindSelectsOnlyEventsWorthInterrupting(t *testing.T) {
	for _, tc := range []struct {
		name       string
		event      servicechat.Event
		wantKind   servicepush.Kind
		wantUrgent bool
		wantOK     bool
	}{
		{
			name:       "agent asks a question",
			event:      servicechat.Event{Type: "tool_use_start", Name: "AskUserQuestion"},
			wantKind:   servicepush.KindQuestion,
			wantUrgent: true,
			wantOK:     true,
		},
		{
			name:   "any other tool call",
			event:  servicechat.Event{Type: "tool_use_start", Name: "Bash"},
			wantOK: false,
		},
		{
			name:     "interactive turn finishes",
			event:    servicechat.Event{Type: "complete"},
			wantKind: servicepush.KindComplete,
			wantOK:   true,
		},
		{
			name:     "interactive run fails",
			event:    servicechat.Event{Type: "error", Message: "boom"},
			wantKind: servicepush.KindError,
			wantOK:   true,
		},
		{
			name:     "scheduled run finishes",
			event:    servicechat.Event{Type: "complete", ScheduledTaskID: "task-1"},
			wantKind: servicepush.KindScheduled,
			wantOK:   true,
		},
		{
			name:     "scheduled run fails",
			event:    servicechat.Event{Type: "error", ScheduledTaskID: "task-1"},
			wantKind: servicepush.KindScheduled,
			wantOK:   true,
		},
		{name: "streaming text", event: servicechat.Event{Type: "assistant_text", Text: "hi"}},
		{name: "reasoning", event: servicechat.Event{Type: "thinking", Text: "hmm"}},
		{name: "tool result", event: servicechat.Event{Type: "tool_use_end"}},
		{name: "user prompt", event: servicechat.Event{Type: "user", Text: "go"}},
		{name: "session bookkeeping", event: servicechat.Event{Type: "session"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind, urgent, ok := notificationKind(tc.event)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if kind != tc.wantKind {
				t.Fatalf("kind = %q, want %q", kind, tc.wantKind)
			}
			if urgent != tc.wantUrgent {
				t.Fatalf("urgent = %v, want %v", urgent, tc.wantUrgent)
			}
		})
	}
}

func TestNotificationTextLeadsWithTheActionAndNamesTheChat(t *testing.T) {
	meta := servicechat.Meta{Title: "Fix the flaky upload test"}

	title, body := notificationText(servicepush.KindQuestion, meta, servicechat.Event{})
	if title != "The agent is asking a question" || body != meta.Title {
		t.Fatalf("question = %q / %q", title, body)
	}

	title, body = notificationText(servicepush.KindError, meta, servicechat.Event{
		Type:    "error",
		Message: "claude exit: status 1",
	})
	if title != "Run failed" || body != "Fix the flaky upload test — claude exit: status 1" {
		t.Fatalf("error = %q / %q", title, body)
	}

	title, _ = notificationText(servicepush.KindScheduled, meta, servicechat.Event{Type: "error"})
	if title != "Scheduled task failed" {
		t.Fatalf("scheduled failure title = %q", title)
	}
	title, _ = notificationText(servicepush.KindScheduled, meta, servicechat.Event{Type: "complete"})
	if title != "Scheduled task finished" {
		t.Fatalf("scheduled success title = %q", title)
	}
}

func TestNotificationTextFallsBackForAnUntitledChat(t *testing.T) {
	_, body := notificationText(servicepush.KindComplete, servicechat.Meta{Title: "   "}, servicechat.Event{})
	if body != "Untitled chat" {
		t.Fatalf("body = %q", body)
	}
}

func TestWithDetailFlattensAndTruncatesAgentOutput(t *testing.T) {
	long := ""
	for len(long) < 200 {
		long += "error "
	}
	got := withDetail("Chat", "line one\nline two")
	if got != "Chat — line one line two" {
		t.Fatalf("got %q", got)
	}
	// The body has to survive an encrypted push payload, so it is bounded.
	if got := withDetail("Chat", long); len([]rune(got)) > 160 {
		t.Fatalf("detail was not truncated: %d runes", len([]rune(got)))
	}
}

func TestANilNotifierIsSafeToCall(t *testing.T) {
	var notifier *chatPushNotifier
	notifier.ChatEvent("beefcafe", servicechat.Event{Type: "complete"})
}
