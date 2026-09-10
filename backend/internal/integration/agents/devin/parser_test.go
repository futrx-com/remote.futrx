package devin

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestParserIsNoOp(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	events, err := parser.ParseLine([]byte(`{"any":"json"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events from no-op parser, got %#v", events)
	}
}

func TestParserHandlesEmptyLine(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	events, err := parser.ParseLine([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events for empty line, got %#v", events)
	}
}

func TestParserSetsDefaultProvider(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "chat-1"})
	if parser.req.Provider != agent.ProviderDevin {
		t.Fatalf("provider = %q, want devin", parser.req.Provider)
	}
}

func TestParserPreservesExplicitProvider(t *testing.T) {
	parser := NewParser(agent.RunRequest{Provider: agent.ProviderDevin, ConversationID: "chat-1"})
	if parser.req.Provider != agent.ProviderDevin {
		t.Fatalf("provider = %q", parser.req.Provider)
	}
}
