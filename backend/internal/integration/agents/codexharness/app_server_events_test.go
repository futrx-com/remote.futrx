package codexharness

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestAppServerNormalizesInclusiveInputUsage(t *testing.T) {
	parser := newAppServerEventParser(agent.RunRequest{
		Provider:       agent.ProviderCodex,
		ConversationID: "chat-1",
		Model:          "gpt-5.6-sol",
	}, "Codex")
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

func TestAppServerPreservesCompletedSubagentMessages(t *testing.T) {
	parser := newAppServerEventParser(agent.RunRequest{
		Provider: agent.ProviderCodex, ConversationID: "chat-1",
	}, "Codex")
	events := parser.ParseNotification("item/completed", json.RawMessage(`{
		"threadId":"parent","turnId":"turn-1","item":{
			"id":"collab-1","type":"collabAgentToolCall","tool":"spawn_agent","status":"completed",
			"senderThreadId":"parent","receiverThreadIds":["child-1"],
			"agentsStates":{"child-1":{"status":"completed","message":"final child report"}}
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventCollaboration {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Native == nil || events[0].Native.ItemID != "collab-1" ||
		!strings.Contains(string(events[0].Data), "final child report") {
		t.Fatalf("collaboration event = %#v", events[0])
	}
}

func TestAppServerKeepsEmptyWaitLifecycleAsHiddenNativeTelemetry(t *testing.T) {
	parser := newAppServerEventParser(agent.RunRequest{
		Provider: agent.ProviderCodex, ConversationID: "chat-1",
	}, "Codex")
	events := parser.ParseNotification("item/completed", json.RawMessage(`{
		"threadId":"parent","turnId":"turn-1","item":{
			"id":"wait-1","type":"collabAgentToolCall","tool":"wait","status":"completed",
			"senderThreadId":"parent","receiverThreadIds":[],"agentsStates":{}
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventProviderNative ||
		events[0].Native == nil || events[0].Native.Method != "item/completed" {
		t.Fatalf("events = %#v", events)
	}
}

func TestAppServerUnknownNotificationHasNativeFallback(t *testing.T) {
	parser := newAppServerEventParser(agent.RunRequest{
		Provider: agent.ProviderCodex, ConversationID: "chat-1",
	}, "Codex")
	events := parser.ParseNotification("future/notification", json.RawMessage(`{"threadId":"thread-1","future":true}`))
	if len(events) != 1 || events[0].Type != agent.EventProviderNative || events[0].Native.Method != "future/notification" {
		t.Fatalf("events = %#v", events)
	}
}

func TestAppServerInterruptedTurnIsNotCompleted(t *testing.T) {
	parser := newAppServerEventParser(agent.RunRequest{Provider: agent.ProviderCodex}, "Codex")
	events := parser.ParseNotification("turn/completed", json.RawMessage(`{"turn":{"id":"turn-1","status":"interrupted"}}`))
	if len(events) != 1 || events[0].Type != agent.EventRunInterrupted {
		t.Fatalf("events = %#v", events)
	}
}

func TestAppServerUsesSelectedProviderInFallbackMessages(t *testing.T) {
	parser := newAppServerEventParser(agent.RunRequest{Provider: agent.ProviderMiniMax}, "MiniMax")
	events := parser.ParseNotification("turn/completed", json.RawMessage(
		`{"turn":{"status":"failed"}}`,
	))
	if len(events) != 1 || events[0].Provider != agent.ProviderMiniMax ||
		events[0].Message != "MiniMax turn failed" {
		t.Fatalf("events = %#v", events)
	}
}
