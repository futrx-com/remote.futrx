package claude

import (
	"github.com/futrx-com/remote.futrx.com/internal/harness"
	"testing"
)

func TestInputAcceptanceRequiresNativeAssistantMessage(t *testing.T) {
	parser := NewParser(harness.RunRequest{TrackInputAcceptance: true})
	events, err := parser.ParseLine([]byte(`{"type":"system","subtype":"init","session_id":"native-session"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == harness.EventInputAccepted {
			t.Fatal("session initialization is not input acceptance")
		}
	}
	events, err = parser.ParseLine([]byte(`{"type":"assistant","session_id":"native-session","uuid":"native-message","message":{"id":"api-id","role":"assistant","content":[{"type":"tool_use","id":"tool-1","name":"read","input":{"path":"x"}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	accepted := 0
	for _, event := range events {
		if event.MessageID != "native-message" {
			t.Fatalf("lost native UUID: %#v", event)
		}
		if event.Native == nil || event.Native.ItemID != "native-message" {
			t.Fatalf("lost native envelope: %#v", event)
		}
		if event.Type == harness.EventInputAccepted {
			accepted++
			if event.SessionID != "native-session" || event.Status == "" {
				t.Fatal(event)
			}
		}
	}
	if accepted != 1 {
		t.Fatalf("acceptance events=%d", accepted)
	}
}

func TestMissingSessionRecoveryDoesNotMatchOtherErrors(t *testing.T) {
	for _, test := range []struct {
		message string
		want    bool
	}{
		{"No conversation found with session ID: native-session", true},
		{"Session not found on upstream server", false},
		{"No conversation found with session ID: other-session", false},
		{"Authentication failed", false},
	} {
		if got := isMissingSessionMessage(test.message, "native-session"); got != test.want {
			t.Errorf("%q: %v", test.message, got)
		}
	}
}

func TestStreamDeltaIdentityUsesMessageNotPerEventUUID(t *testing.T) {
	parser := NewParser(harness.RunRequest{TrackInputAcceptance: true, ResumeID: "session"})
	if _, err := parser.ParseLine([]byte(`{"type":"stream_event","uuid":"start-event","event":{"type":"message_start","message":{"id":"stable-message"}}}`)); err != nil {
		t.Fatal(err)
	}
	for _, uuid := range []string{"first-delta", "second-delta"} {
		events, err := parser.ParseLine([]byte(`{"type":"stream_event","uuid":"` + uuid + `","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"fragment"}}}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 1 || events[0].MessageID != "stable-message" || events[0].Native == nil || events[0].Native.ItemID != uuid {
			t.Fatalf("identity lost: %#v", events)
		}
	}
}
