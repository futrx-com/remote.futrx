package codex

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestParserMapsThreadStarted(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	events, err := parser.ParseLine([]byte(`{"type":"thread.started","thread_id":"thread-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != agent.EventSessionUpdated || events[0].SessionID != "thread-1" || events[0].Provider != agent.ProviderCodex {
		t.Fatalf("unexpected event: %#v", events[0])
	}
}

func TestParserMapsAgentMessageItemAsDelta(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	events, err := parser.ParseLine([]byte(`{"type":"item.completed","item":{"id":"msg-1","type":"agent_message","text":"hello"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != agent.EventAssistantTextDelta || events[0].Text != "hello" || events[0].ItemKind != agent.ItemMessage {
		t.Fatalf("unexpected event: %#v", events[0])
	}
}

func TestParserEmitsOnlyNewTextForUpdatedAgentMessage(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	first, err := parser.ParseLine([]byte(`{"type":"item.updated","item":{"id":"msg-1","type":"agent_message","text":"hello"}}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := parser.ParseLine([]byte(`{"type":"item.completed","item":{"id":"msg-1","type":"agent_message","text":"hello world"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Text != "hello" {
		t.Fatalf("unexpected first events: %#v", first)
	}
	if len(second) != 1 || second[0].Text != " world" {
		t.Fatalf("unexpected second events: %#v", second)
	}
}

func TestParserMapsCommandToolLifecycle(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	start, err := parser.ParseLine([]byte(`{"type":"item.started","item":{"id":"cmd-1","type":"command_execution","command":"go test ./...","status":"in_progress"}}`))
	if err != nil {
		t.Fatal(err)
	}
	end, err := parser.ParseLine([]byte(`{"type":"item.completed","item":{"id":"cmd-1","type":"command_execution","command":"go test ./...","aggregated_output":"ok\n","exit_code":0,"status":"completed"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(start) != 1 || start[0].Type != agent.EventToolStarted || start[0].ToolName != "Bash" {
		t.Fatalf("unexpected start event: %#v", start)
	}
	if len(end) != 1 || end[0].Type != agent.EventToolCompleted || end[0].Output != "ok\n" || end[0].IsError {
		t.Fatalf("unexpected end event: %#v", end)
	}
}

func TestParserMapsTurnCompletedUsage(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	events, err := parser.ParseLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":3,"output_tokens":4,"reasoning_output_tokens":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != agent.EventRunCompleted {
		t.Fatalf("unexpected events: %#v", events)
	}
	if string(events[0].Usage) != `{"cache_read_input_tokens":3,"input_tokens":10,"output_tokens":4,"reasoning_output_tokens":2}` {
		t.Fatalf("unexpected usage: %s", events[0].Usage)
	}
}

func TestParserMapsTurnFailed(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	events, err := parser.ParseLine([]byte(`{"type":"turn.failed","error":{"message":"bad auth"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != agent.EventRunFailed || events[0].Message != "bad auth" || !events[0].IsError {
		t.Fatalf("unexpected events: %#v", events)
	}
}
