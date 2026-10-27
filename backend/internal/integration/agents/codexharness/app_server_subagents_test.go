package codexharness

import (
	"encoding/json"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestAppServerSubagentTrackerCapturesMCPToolDetails(t *testing.T) {
	tracker := newAppServerSubagentTracker(newAppServerEventParser(agent.RunRequest{
		Provider:       agent.ProviderCodex,
		ConversationID: "chat-1",
	}, "Codex"))
	tracker.ParseNotification("parent", "item/started", json.RawMessage(`{
		"threadId":"child","turnId":"child-turn","item":{
			"id":"tool-1","type":"mcpToolCall","server":"files","tool":"read",
			"status":"inProgress","arguments":{"path":"/workspace/file.txt"}
		}
	}`))
	events := tracker.ParseNotification("parent", "item/completed", json.RawMessage(`{
		"threadId":"child","turnId":"child-turn","item":{
			"id":"tool-1","type":"mcpToolCall","server":"files","tool":"read",
			"status":"completed","arguments":{"path":"/workspace/file.txt"},
			"result":{"content":"complete output"}
		}
	}`))
	if len(events) != 1 || events[0].Type != agent.EventCollaboration {
		t.Fatalf("events = %#v", events)
	}
	var data struct {
		Tools []appServerSubagentTool `json:"tools"`
	}
	if err := json.Unmarshal(events[0].Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Tools) != 1 {
		t.Fatalf("tools = %#v", data.Tools)
	}
	tool := data.Tools[0]
	var input map[string]any
	if err := json.Unmarshal(tool.Input, &input); err != nil {
		t.Fatal(err)
	}
	if tool.ID != "tool-1" || tool.Name != "files/read" || tool.Status != "completed" ||
		input["path"] != "/workspace/file.txt" || tool.Output != `{"content":"complete output"}` ||
		tool.IsError || tool.StartedAt == 0 || tool.CompletedAt < tool.StartedAt || tool.DurationMs == nil {
		t.Fatalf("tool = %#v, input = %#v", tool, input)
	}
}
