package kimi

import (
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	"testing"
)

func TestInputAcceptanceRequiresOutputAndResumeHint(t *testing.T) {
	for _, withOutput := range []bool{false, true} {
		parser := NewParser(harness.RunRequest{TrackInputAcceptance: true})
		if withOutput {
			if _, err := parser.ParseLine([]byte(`{"role":"assistant","content":"done"}`)); err != nil {
				t.Fatal(err)
			}
		}
		events, err := parser.ParseLine([]byte(`{"role":"meta","type":"session.resume_hint","session_id":"native"}`))
		if err != nil {
			t.Fatal(err)
		}
		accepted := false
		for _, event := range events {
			if event.Type == harness.EventInputAccepted {
				accepted = true
				if event.SessionID != "native" {
					t.Fatal(event)
				}
			}
		}
		if accepted != withOutput {
			t.Fatalf("withOutput=%v accepted=%v", withOutput, accepted)
		}
	}
}
