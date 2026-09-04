package prompt

import (
	"errors"
	"testing"
)

type blockedStartGate bool

func (g blockedStartGate) Blocked() bool { return bool(g) }

func TestStartRejectsPromptDuringWorkspaceMaintenance(t *testing.T) {
	service := New(nil, nil, nil, nil, nil, WithStartGate(blockedStartGate(true)))
	var transient ChatEvent
	_, err := service.Start(StartInput{ChatID: "aabbcc11", Prompt: "hello"}, func(event ChatEvent) {
		transient = event
	})
	if !errors.Is(err, ErrMaintenance) {
		t.Fatalf("Start error = %v, want ErrMaintenance", err)
	}
	if transient.Type != "error" || transient.Message != ErrMaintenance.Error() {
		t.Fatalf("transient event = %+v", transient)
	}
}
