package codex

import (
	"encoding/json"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestAppServerNormalizesInclusiveInputUsage(t *testing.T) {
	parser := newAppServerEventParser(agent.RunRequest{
		ConversationID: "chat-1",
		Model:          "gpt-5.6-sol",
	})
	parser.ParseNotification("thread/tokenUsage/updated", json.RawMessage(`{
		"tokenUsage":{"last":{
			"inputTokens":10,
			"cachedInputTokens":3,
			"cacheWriteInputTokens":2,
			"outputTokens":4,
			"reasoningOutputTokens":2
		}}
	}`))
	events := parser.ParseNotification("turn/completed", json.RawMessage(
		`{"turn":{"status":"completed"}}`,
	))
	if len(events) != 1 || events[0].Type != agent.EventRunCompleted {
		t.Fatalf("events = %#v", events)
	}
	usage, ok := agent.ParseUsage(events[0].Usage)
	if !ok {
		t.Fatalf("usage not parsed from %s", events[0].Usage)
	}
	if usage.InputTokens != 5 || usage.CacheReadTokens != 3 ||
		usage.CacheWriteTokens != 2 || usage.OutputTokens != 4 {
		t.Fatalf("usage = %#v", usage)
	}
	if usage.Model != "gpt-5.6-sol" {
		t.Fatalf("model = %q, want gpt-5.6-sol", usage.Model)
	}
	if usage.TotalTokens() != 14 {
		t.Fatalf("total tokens = %d, want 14", usage.TotalTokens())
	}
}
