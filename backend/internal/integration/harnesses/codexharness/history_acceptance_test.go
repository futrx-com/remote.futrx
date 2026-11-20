package codexharness

import (
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	"testing"
)

func TestTurnStartResponseCarriesAcceptanceEvidence(t *testing.T) {
	var events []harness.Event
	run := &appServerRun{req: harness.RunRequest{Provider: harness.ProviderCodex, TrackInputAcceptance: true}, threadID: "native-thread", emit: func(e harness.Event) { events = append(events, e) }}
	run.handleResponse(appServerTurnRequestID, []byte(`{"turn":{"id":"native-turn"}}`))
	if len(events) != 1 || events[0].Type != harness.EventInputAccepted {
		t.Fatalf("events=%#v", events)
	}
	if events[0].SessionID != "native-thread" || events[0].Native == nil || events[0].Native.TurnID != "native-turn" {
		t.Fatalf("missing correlation=%#v", events[0])
	}
}
