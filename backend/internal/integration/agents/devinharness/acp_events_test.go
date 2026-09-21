package devinharness

import (
	"encoding/json"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func newTestParser() *acpEventParser {
	return newACPEventParser(agent.RunRequest{
		Provider:       agent.ProviderDevin,
		ConversationID: "chat-1",
	}, "Devin")
}

func TestACPAgentMessageChunk(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"agent_message_chunk",
			"content":{"type":"text","text":"Hello world"},
			"messageId":"msg-1"
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventAssistantTextDelta {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Text != "Hello world" {
		t.Fatalf("text = %q", events[0].Text)
	}
	if events[0].MessageID != "msg-1" {
		t.Fatalf("messageId = %q", events[0].MessageID)
	}
	if events[0].ItemKind != agent.ItemMessage {
		t.Fatalf("itemKind = %q", events[0].ItemKind)
	}
}

func TestACPAgentThoughtChunk(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"agent_thought_chunk",
			"content":{"type":"text","text":"Thinking..."},
			"messageId":"msg-1"
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventReasoningDelta {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Text != "Thinking..." {
		t.Fatalf("text = %q", events[0].Text)
	}
}

func TestACPUserMessageChunkIsSilent(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"user_message_chunk",
			"content":{"type":"text","text":"user input echo"}
		}
	}`))
	if len(events) != 0 {
		t.Fatalf("user_message_chunk should produce no events, got %#v", events)
	}
}

func TestACPToolCallStarted(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"tool_call",
			"toolCallId":"tc-1",
			"title":"Bash",
			"kind":"command",
			"rawInput":{"command":"ls -la"},
			"status":"inProgress"
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventToolStarted {
		t.Fatalf("events = %#v", events)
	}
	if events[0].ItemID != "tc-1" {
		t.Fatalf("itemId = %q", events[0].ItemID)
	}
	if events[0].ToolName != "Bash" {
		t.Fatalf("toolName = %q", events[0].ToolName)
	}
	if string(events[0].Input) == "" || events[0].ItemKind != agent.ItemToolCall {
		t.Fatalf("input = %q, itemKind = %q", events[0].Input, events[0].ItemKind)
	}
}

func TestACPToolCallUpdateCompleted(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"tool_call_update",
			"toolCallId":"tc-1",
			"status":"completed",
			"content":{"type":"text","text":"file1.txt\nfile2.txt"}
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventToolCompleted {
		t.Fatalf("events = %#v", events)
	}
	if events[0].ItemID != "tc-1" {
		t.Fatalf("itemId = %q", events[0].ItemID)
	}
	if events[0].Output != "file1.txt\nfile2.txt" {
		t.Fatalf("output = %q", events[0].Output)
	}
	if events[0].IsError {
		t.Fatal("completed should not be error")
	}
}

func TestACPToolCallUpdateFailed(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"tool_call_update",
			"toolCallId":"tc-1",
			"status":"failed",
			"content":{"type":"text","text":"command not found"}
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventToolCompleted {
		t.Fatalf("events = %#v", events)
	}
	if !events[0].IsError {
		t.Fatal("failed status should set IsError")
	}
	if events[0].Output != "command not found" {
		t.Fatalf("output = %q", events[0].Output)
	}
}

func TestACPToolCallUpdateNonTerminalIsSilent(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"tool_call_update",
			"toolCallId":"tc-1",
			"status":"inProgress"
		}
	}`))
	if len(events) != 0 {
		t.Fatalf("non-terminal tool_call_update should produce no events, got %#v", events)
	}
}

func TestACPPlanIsNative(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"plan",
			"entries":[{"step":"1","description":"do thing"}]
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventProviderNative {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Native == nil || events[0].Native.Method != "plan" {
		t.Fatalf("native = %#v", events[0].Native)
	}
}

func TestACPAvailableCommandsUpdateIsNative(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"available_commands_update",
			"availableCommands":[{"id":"cmd-1","name":"test"}]
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventProviderNative {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Native == nil || events[0].Native.Method != "available_commands_update" {
		t.Fatalf("native = %#v", events[0].Native)
	}
}

func TestACPCurrentModeUpdateIsNative(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"current_mode_update",
			"currentModeId":"plan"
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventProviderNative {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Native == nil || events[0].Native.Method != "current_mode_update" {
		t.Fatalf("native = %#v", events[0].Native)
	}
}

func TestACPUnknownSessionUpdateIsNative(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"future_variant",
			"custom":"data"
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventProviderNative {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Native == nil || events[0].Native.Method != "future_variant" {
		t.Fatalf("native = %#v", events[0].Native)
	}
}

func TestACPCognitionOutputIsSilent(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("_cognition.ai/output", json.RawMessage(`{"message":"MCP connected"}`))
	if len(events) != 0 {
		t.Fatalf("_cognition.ai/output should produce no events, got %#v", events)
	}
}

func TestACPCognitionMCPServersChangedIsSilent(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("_cognition.ai/mcp/serversChanged", json.RawMessage(`{"servers":[]}`))
	if len(events) != 0 {
		t.Fatalf("_cognition.ai/mcp/serversChanged should produce no events, got %#v", events)
	}
}

func TestACPCognitionDocumentIsSilent(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("_cognition.ai/document/opened", json.RawMessage(`{"path":"/workspace/file.go"}`))
	if len(events) != 0 {
		t.Fatalf("_cognition.ai/document/* should produce no events, got %#v", events)
	}
}

func TestACPCognitionOtherIsNative(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("_cognition.ai/customExtension", json.RawMessage(`{"data":true}`))
	if len(events) != 1 || events[0].Type != agent.EventProviderNative {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Native == nil || events[0].Native.Method != "_cognition.ai/customExtension" {
		t.Fatalf("native = %#v", events[0].Native)
	}
}

func TestACPUnknownNotificationIsNative(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("future/method", json.RawMessage(`{"data":true}`))
	if len(events) != 1 || events[0].Type != agent.EventProviderNative {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Native == nil || events[0].Native.Method != "future/method" {
		t.Fatalf("native = %#v", events[0].Native)
	}
}

func TestACPMalformedSessionUpdateIsNative(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`not valid json`))
	if len(events) != 1 || events[0].Type != agent.EventProviderNative {
		t.Fatalf("malformed session/update should fall back to native, got %#v", events)
	}
}

func TestACPContentArrayTextExtraction(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"agent_message_chunk",
			"content":[{"type":"text","text":"part1"},{"type":"text","text":"part2"}],
			"messageId":"msg-1"
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventAssistantTextDelta {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Text != "part1part2" {
		t.Fatalf("text = %q, want part1part2", events[0].Text)
	}
}

func TestACPProviderLabelInEvents(t *testing.T) {
	parser := newTestParser()
	events := parser.ParseNotification("session/update", json.RawMessage(`{
		"sessionId":"s-1",
		"update":{
			"sessionUpdate":"agent_message_chunk",
			"content":{"type":"text","text":"hi"},
			"messageId":"msg-1"
		}
	}`))
	if len(events) != 1 || events[0].Provider != agent.ProviderDevin {
		t.Fatalf("provider = %q, want devin", events[0].Provider)
	}
	if events[0].ConversationID != "chat-1" {
		t.Fatalf("conversationId = %q", events[0].ConversationID)
	}
}
