package service

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

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
			name:     "application turn finishes",
			event:    servicechat.Event{Type: "complete", ApplicationID: "build-monitor"},
			wantKind: servicepush.KindApplication,
			wantOK:   true,
		},
		{
			name:     "application turn fails",
			event:    servicechat.Event{Type: "error", ApplicationID: "build-monitor"},
			wantKind: servicepush.KindApplication,
			wantOK:   true,
		},
		{
			name:     "scheduled tasks uses the application contract",
			event:    servicechat.Event{Type: "complete", ApplicationID: "scheduled-tasks"},
			wantKind: servicepush.KindApplication,
			wantOK:   true,
		},
		{
			name:     "application identity takes precedence over legacy marker",
			event:    servicechat.Event{Type: "complete", ApplicationID: "build-monitor", ScheduledTaskID: "old-task"},
			wantKind: servicepush.KindApplication,
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
	if title != "Upload project - Agent needs your answer ❓" || body != "Open the chat to answer the question." {
		t.Fatalf("question = %q / %q", title, body)
	}

	title, body = notificationText(servicepush.KindError, projectName, servicechat.Event{
		Type:    "error",
		Message: "claude exit: status 1",
	})
	if title != "Upload project - Agent encountered an error ❌" || body != "claude exit: status 1" {
		t.Fatalf("error = %q / %q", title, body)
	}

	title, body = notificationText(servicepush.KindScheduled, projectName, servicechat.Event{Type: "error"})
	if title != "Upload project - Agent encountered an error ❌" || body != "Open the chat to review the error." {
		t.Fatalf("scheduled failure = %q / %q", title, body)
	}
	title, body = notificationText(servicepush.KindScheduled, projectName, servicechat.Event{Type: "complete", NotificationSummary: "Fixed the upload test"})
	if title != "Upload project - Agent finished ✅" || body != "Fixed the upload test" {
		t.Fatalf("scheduled success = %q / %q", title, body)
	}
}

func TestNotificationTextFallsBackWithoutSummaryOrProject(t *testing.T) {
	title, body := notificationText(servicepush.KindComplete, "   ", servicechat.Event{})
	if title != "Remote - Agent finished ✅" || body != "Open the chat to see the result." {
		t.Fatalf("notification = %q / %q", title, body)
	}
}

func TestApplicationNotificationTextUsesNeutralFallbackAndResults(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event servicechat.Event
		title string
		body  string
	}{
		{"success without summary", servicechat.Event{Type: "complete"}, "Remote - Agent finished ✅", "An application-started turn finished."},
		{"success with summary", servicechat.Event{Type: "complete", NotificationSummary: "Build passed"}, "Remote - Agent finished ✅", "Build passed"},
		{"failure without detail", servicechat.Event{Type: "error"}, "Remote - Agent encountered an error ❌", "Open the chat to review the error."},
		{"failure with detail", servicechat.Event{Type: "error", Message: "build failed"}, "Remote - Agent encountered an error ❌", "build failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			title, body := notificationText(servicepush.KindApplication, "Remote", tc.event)
			if title != tc.title || body != tc.body {
				t.Fatalf("application notification = %q / %q, want %q / %q", title, body, tc.title, tc.body)
			}
		})
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

func TestNativeQuestionKindsExcludeApprovals(t *testing.T) {
	for _, status := range []string{"user_input", "approval", "permission", "elicitation", "provider_request"} {
		kind, urgent, ok := notificationKind(servicechat.Event{Type: "interaction_request", Status: status})
		if status == "user_input" {
			if kind != servicepush.KindQuestion || !urgent || !ok {
				t.Fatalf("native question classification = %q, %v, %v", kind, urgent, ok)
			}
		} else if ok {
			t.Fatalf("non-question interaction notified: %s", status)
		}
	}
}

func TestQuestionBodySummarizesTheBatchAndHandlesInvalidInput(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`{"questions":[{"question":"Choose a region?"},{"question":"How many\nreplicas?"}]}`, "Choose a region? · How many replicas?"},
		{`{"questions":[{"question":"private question","isSecret":true},{"question":"Choose a region?"}]}`, "Choose a region?"},
		{`{"questions":[{"question":"private question","isSecret":true}]}`, "Open the chat to answer the question."},
		{`{"questions":[]}`, "Open the chat to answer the question."},
		{`{"questions":[{"question":"  "}]}`, "Open the chat to answer the question."},
		{`{"questions":[{"question":5}]}`, "Open the chat to answer the question."},
		{`broken`, "Open the chat to answer the question."},
	} {
		if got := questionBody(servicechat.Event{Input: json.RawMessage(tc.input)}); got != tc.want {
			t.Fatalf("question body = %q, want %q", got, tc.want)
		}
	}
	for _, summary := range []string{strings.Repeat("a", 500), strings.Repeat("🌍", 500)} {
		input, err := json.Marshal(map[string]any{"questions": []map[string]string{{"question": summary}}})
		if err != nil {
			t.Fatal(err)
		}
		body := questionBody(servicechat.Event{Input: input})
		if len([]rune(body)) > 200 || len(body) > 240 || !utf8.ValidString(body) {
			t.Fatalf("invalid bounded question body: %q", body)
		}
	}
}

func TestPlainQuestionSummaryUsesNormalCompletionNotification(t *testing.T) {
	event := servicechat.Event{Type: "complete", NotificationSummary: "Choose the deployment region and replica count."}
	kind, urgent, ok := notificationKind(event)
	title, body := notificationText(kind, "Project", event)
	if !ok || urgent || kind != servicepush.KindComplete || title != "Project - Agent finished ✅" || body != event.NotificationSummary {
		t.Fatalf("plain question notification = %q, %v, %q, %q", kind, urgent, title, body)
	}
}
