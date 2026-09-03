package prompt

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
)

func TestChatEventFromAgentEventKeepsProviderlessSessionGeneric(t *testing.T) {
	ev, ok := chatEventFromAgentEvent(agent.Event{
		T:         123,
		Type:      agent.EventSessionUpdated,
		SessionID: "claude-session",
	})
	if !ok {
		t.Fatal("expected event to map")
	}
	if ev.Type != "session" || ev.SessionID != "claude-session" || ev.Provider != "" || ev.ClaudeSessionID != "" || ev.T != 123 {
		t.Fatalf("unexpected chat event: %#v", ev)
	}
}

func TestEmitAgentEventUsesSelectedProviderWhenAdapterOmitsIt(t *testing.T) {
	ctx := context.Background()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(ctx, servicechat.Meta{
		ID: "abcdef123456", Provider: "future-agent",
	})
	if err != nil {
		t.Fatal(err)
	}

	service := &Service{store: store}
	var emitted ChatEvent
	service.emitAgentEvent(ctx, meta.ID, "future-agent", agent.Event{
		Type: agent.EventSessionUpdated, SessionID: "future-session",
	}, func(event ChatEvent) {
		emitted = event
	})

	stored, err := store.Get(ctx, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SessionID("future-agent") != "future-session" || stored.ClaudeSessionID != "" {
		t.Fatalf("stored sessions = %#v, claude = %q", stored.Sessions, stored.ClaudeSessionID)
	}
	if emitted.Provider != "future-agent" || emitted.SessionID != "future-session" || emitted.ClaudeSessionID != "" {
		t.Fatalf("emitted session = %#v", emitted)
	}
}

func TestChatEventFromAgentEventMapsCodexSession(t *testing.T) {
	ev, ok := chatEventFromAgentEvent(agent.Event{
		T:         123,
		Type:      agent.EventSessionUpdated,
		Provider:  agent.ProviderCodex,
		SessionID: "codex-thread",
	})
	if !ok {
		t.Fatal("expected event to map")
	}
	if ev.Type != "session" || ev.SessionID != "codex-thread" || ev.CodexSessionID != "codex-thread" || ev.Provider != servicechat.ProviderCodex {
		t.Fatalf("unexpected chat event: %#v", ev)
	}
}

func TestChatEventFromAgentEventPersistsCompletionProvider(t *testing.T) {
	ev, ok := chatEventFromAgentEvent(agent.Event{
		T:        123,
		Type:     agent.EventRunCompleted,
		Provider: "future-agent",
		Usage:    json.RawMessage(`{"schema_version":1,"input_tokens":7}`),
	})
	if !ok {
		t.Fatal("expected event to map")
	}
	if ev.Type != "complete" || ev.Provider != "future-agent" ||
		string(ev.Usage) != `{"schema_version":1,"input_tokens":7}` {
		t.Fatalf("unexpected completion event: %#v", ev)
	}
}

func TestChatEventFromAgentEventMapsToolLifecycle(t *testing.T) {
	input := json.RawMessage(`{"cmd":"go test ./..."}`)
	start, ok := chatEventFromAgentEvent(agent.Event{
		T:        456,
		Type:     agent.EventToolStarted,
		ItemID:   "tool-1",
		ToolName: "Bash",
		Input:    input,
	})
	if !ok {
		t.Fatal("expected start event to map")
	}
	if start.Type != "tool_use_start" || start.ID != "tool-1" || start.Name != "Bash" || string(start.Input) != string(input) {
		t.Fatalf("unexpected start event: %#v", start)
	}

	end, ok := chatEventFromAgentEvent(agent.Event{
		T:      789,
		Type:   agent.EventToolCompleted,
		ItemID: "tool-1",
		Output: "ok",
	})
	if !ok {
		t.Fatal("expected end event to map")
	}
	if end.Type != "tool_use_end" || end.ID != "tool-1" || end.Output != "ok" || end.IsError {
		t.Fatalf("unexpected end event: %#v", end)
	}
}
