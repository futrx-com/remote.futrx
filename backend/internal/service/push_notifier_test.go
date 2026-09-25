package service

import (
	"encoding/json"
	"strings"
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

func TestNotificationTextUsesProjectTitleAndResult(t *testing.T) {
	projectName := "Upload project"

	title, body := notificationText(servicepush.KindQuestion, projectName, servicechat.Event{})
	if title != "Upload project - Agent needs your answer" || body != "Open the chat to answer the question." {
		t.Fatalf("question = %q / %q", title, body)
	}

	title, body = notificationText(servicepush.KindError, projectName, servicechat.Event{
		Type:    "error",
		Message: "claude exit: status 1",
	})
	if title != "Upload project - Agent encountered an error" || body != "claude exit: status 1" {
		t.Fatalf("error = %q / %q", title, body)
	}

	title, body = notificationText(servicepush.KindScheduled, projectName, servicechat.Event{Type: "error"})
	if title != "Upload project - Agent encountered an error" || body != "Open the chat to review the error." {
		t.Fatalf("scheduled failure = %q / %q", title, body)
	}
	title, body = notificationText(servicepush.KindScheduled, projectName, servicechat.Event{Type: "complete", NotificationSummary: "Fixed the upload test"})
	if title != "Upload project - Agent finished" || body != "Fixed the upload test" {
		t.Fatalf("scheduled success = %q / %q", title, body)
	}
}

func TestNotificationTextFallsBackWithoutSummaryOrProject(t *testing.T) {
	title, body := notificationText(servicepush.KindComplete, "   ", servicechat.Event{})
	if title != "Remote - Agent finished" || body != "Open the chat to see the result." {
		t.Fatalf("notification = %q / %q", title, body)
	}
}

func TestNotificationPayloadStaysSmallWithLongUnicodeNames(t *testing.T) {
	title, body := notificationText(servicepush.KindComplete,
		strings.Repeat("世界", 100),
		servicechat.Event{NotificationSummary: strings.Repeat("🌍", 200)},
	)
	payload, err := json.Marshal(servicepush.Notification{
		Kind: servicepush.KindComplete, Title: title, Body: body,
		ChatID: "abcdef12", Tag: "chat:abcdef12",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > 600 {
		t.Fatalf("push payload is %d bytes", len(payload))
	}
}

func TestErrorBodyFlattensAndTruncatesAgentOutput(t *testing.T) {
	long := ""
	for len(long) < 200 {
		long += "error "
	}
	got := errorBody("line one\nline two")
	if got != "line one line two" {
		t.Fatalf("got %q", got)
	}
	// The body has to survive an encrypted push payload, so it is bounded.
	if got := errorBody(long); len([]byte(got)) > 180 {
		t.Fatalf("detail was not truncated: %d bytes", len(got))
	}
}

func TestANilNotifierIsSafeToCall(t *testing.T) {
	var notifier *chatPushNotifier
	notifier.ChatEvent("beefcafe", servicechat.Event{Type: "complete"})
}
