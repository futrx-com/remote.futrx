package flow

import (
	"encoding/json"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func TestFlowMapper_BuildState(t *testing.T) {
	mapper := NewFlowMapper()

	inputNav, _ := json.Marshal(map[string]string{"url": "https://example.com"})
	inputClick, _ := json.Marshal(map[string]string{"ref": "e12"})

	events := []servicechat.Event{
		{
			Seq:  1,
			Type: "user",
			Text: "Open example.com and click the login button",
		},
		{
			Seq:      2,
			Type:     "tool_call",
			ToolName: "browser_navigate",
			Input:    inputNav,
		},
		{
			Seq:      3,
			Type:     "tool_call",
			ToolName: "browser_click",
			Input:    inputClick,
			Output:   "Clicked element e12",
		},
	}

	state := mapper.BuildState("test_chat_1", events)

	if state.ChatID != "test_chat_1" {
		t.Errorf("expected ChatID test_chat_1, got %s", state.ChatID)
	}

	if len(state.Checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(state.Checkpoints))
	}

	if len(state.Nodes) != 2 {
		t.Fatalf("expected 2 order nodes, got %d", len(state.Nodes))
	}

	if state.Nodes[0].Verb != "navigate" || state.Nodes[0].Icon != "🌐" {
		t.Errorf("expected node 0 navigate icon, got verb %s icon %s", state.Nodes[0].Verb, state.Nodes[0].Icon)
	}

	if state.Nodes[1].Verb != "click" || state.Nodes[1].Icon != "🖱️" {
		t.Errorf("expected node 1 click icon, got verb %s icon %s", state.Nodes[1].Verb, state.Nodes[1].Icon)
	}

	if len(state.Edges) != 1 {
		t.Fatalf("expected 1 edge between node 0 and node 1, got %d", len(state.Edges))
	}

	if state.Nodes[0].BinaryPath == "" || state.Nodes[1].BinaryPath == "" {
		t.Errorf("expected binary path for Maestro node, got node0: %s, node1: %s", state.Nodes[0].BinaryPath, state.Nodes[1].BinaryPath)
	}
}
