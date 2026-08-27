package claude

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestParserMapsTextDelta(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	events, err := parser.ParseLine([]byte(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != agent.EventAssistantTextDelta || events[0].Text != "hello" || events[0].ConversationID != "chat-1" {
		t.Fatalf("unexpected event: %#v", events[0])
	}
}

func TestParserMapsSessionAndComplete(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	events, err := parser.ParseLine([]byte(`{"type":"result","session_id":"session-1","model":"sonnet","usage":{"input_tokens":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected session and completion events, got %d", len(events))
	}
	if events[0].Type != agent.EventSessionUpdated || events[0].SessionID != "session-1" || events[0].Model != "sonnet" {
		t.Fatalf("unexpected session event: %#v", events[0])
	}
	if events[1].Type != agent.EventRunCompleted || string(events[1].Usage) != `{"input_tokens":1}` {
		t.Fatalf("unexpected completion event: %#v", events[1])
	}
}

func TestParserMapsClaudeResultErrorAsRunFailed(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	events, err := parser.ParseLine([]byte(`{"type":"result","is_error":true,"result":"Failed to authenticate","usage":{"input_tokens":0}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected failure event, got %d", len(events))
	}
	if events[0].Type != agent.EventRunFailed || events[0].Message != "Failed to authenticate" || !events[0].IsError {
		t.Fatalf("unexpected failure event: %#v", events[0])
	}
}

func TestParserMapsToolLifecycle(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	start, err := parser.ParseLine([]byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tool-1","name":"Bash","input":{"cmd":"go test ./..."}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(start) != 1 || start[0].Type != agent.EventToolStarted || start[0].ItemID != "tool-1" || start[0].ToolName != "Bash" {
		t.Fatalf("unexpected start event: %#v", start)
	}

	end, err := parser.ParseLine([]byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tool-1","content":[{"type":"text","text":"ok"}]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(end) != 1 || end[0].Type != agent.EventToolCompleted || end[0].ItemID != "tool-1" || end[0].Output != "ok" {
		t.Fatalf("unexpected end event: %#v", end)
	}
}

func TestParserReturnsJSONErrors(t *testing.T) {
	parser := NewParser(agent.RunRequest{})
	if _, err := parser.ParseLine([]byte(`not json`)); err == nil {
		t.Fatal("expected JSON parse error")
	}
}
