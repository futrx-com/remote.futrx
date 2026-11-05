package kimi

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNativeShutdownDrainsFinalFramesBeforeAcceptingExit(t *testing.T) {
	for _, kind := range []string{"fatal", "event"} {
		t.Run(kind, func(t *testing.T) {
			for range 100 {
				p := &serverTransport{
					stdin:   fixtureWriter(func([]byte) error { return nil }),
					frames:  make(chan bridgeFrame, 1),
					done:    make(chan error, 1),
					onEvent: func(json.RawMessage) error { return errors.New("late failure") },
				}
				p.frames <- bridgeFrame{Type: kind, Message: "late failure"}
				close(p.frames)
				p.done <- nil
				if err := p.close(); err == nil || !strings.Contains(err.Error(), "late failure") {
					t.Fatalf("shutdown lost the final %s: %v", kind, err)
				}
			}
		})
	}
}
