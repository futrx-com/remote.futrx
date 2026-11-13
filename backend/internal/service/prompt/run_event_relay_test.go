package prompt

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
)

type collaborationPromptProvider struct {
	events []agent.Event
}

func (p *collaborationPromptProvider) ID() agent.ProviderID { return agent.ProviderCodex }

func (p *collaborationPromptProvider) Parser(agent.RunRequest) agent.LineParser { return nil }

func (p *collaborationPromptProvider) Capabilities(context.Context, agent.CapabilityRequest) (agent.Capabilities, error) {
	return agent.Capabilities{Provider: agent.ProviderCodex}, nil
}

func (p *collaborationPromptProvider) Run(
	_ context.Context,
	_ agent.RunRequest,
	emit func(agent.Event),
) error {
	for _, ev := range p.events {
		emit(ev)
	}
	return nil
}

func TestStripSubagentNotificationSummariesLeavesCleanPayloadBytes(t *testing.T) {
	clean := json.RawMessage(`{"type":"subagentThread","agentsStates":{"child-1":{"status":"completed","message":"Clean reply"}},"toolCount":1}`)
	if got := stripSubagentNotificationSummaries(clean); string(got) != string(clean) {
		t.Fatalf("clean payload was rewritten: %s", got)
	}
}

func TestStripSubagentNotificationSummariesRemovesTrailerFromEveryMessage(t *testing.T) {
	dirty := json.RawMessage(`{"type":"subagentThread",
		"receiverThreadIds":["child-1","child-2"],
		"agentsStates":{
			"child-1":{"status":"completed","message":"Fixed the route. <notification_summary>Chat links survive refresh.</notification_summary>"},
			"child-2":{"status":"completed","message":"Second reply <notification_summary>private</notification_summary>"},
			"child-3":{"status":"inProgress"}
		},
		"toolCount":2,"failedToolCount":0}`)
	cleaned := stripSubagentNotificationSummaries(dirty)
	if string(cleaned) == string(dirty) {
		t.Fatal("dirty payload came back unchanged")
	}
	var payload struct {
		ReceiverThreadIDs []string `json:"receiverThreadIds"`
		ToolCount         int      `json:"toolCount"`
		AgentsStates      map[string]struct {
			Status  string  `json:"status"`
			Message *string `json:"message"`
		} `json:"agentsStates"`
	}
	if err := json.Unmarshal(cleaned, &payload); err != nil {
		t.Fatalf("cleaned payload is not valid JSON: %v", err)
	}
	wantMessage := "Fixed the route. "
	if payload.AgentsStates["child-1"].Message == nil || *payload.AgentsStates["child-1"].Message != wantMessage {
		t.Fatalf("child-1 message = %v, want %q", payload.AgentsStates["child-1"].Message, wantMessage)
	}
	if payload.AgentsStates["child-2"].Message == nil || *payload.AgentsStates["child-2"].Message != "Second reply " {
		t.Fatalf("child-2 message = %v", *payload.AgentsStates["child-2"].Message)
	}
	if payload.AgentsStates["child-3"].Message != nil || payload.AgentsStates["child-3"].Status != "inProgress" {
		t.Fatalf("child-3 state = %+v, want no message and status inProgress", payload.AgentsStates["child-3"])
	}
	if len(payload.ReceiverThreadIDs) != 2 || payload.ToolCount != 2 {
		t.Fatalf("sibling fields changed: receiverThreadIds = %v, toolCount = %d", payload.ReceiverThreadIDs, payload.ToolCount)
	}
}

func TestStripSubagentNotificationSummariesHandlesEscapedMarkers(t *testing.T) {
	// The harness serializes collaboration data with json.Marshal, which
	// escapes the marker's angle brackets in the event payload.
	dirty, err := json.Marshal(map[string]any{
		"type": "subagentThread",
		"agentsStates": map[string]any{
			"child-1": map[string]any{
				"status":  "completed",
				"message": "Fixed the route. <notification_summary>Chat links survive refresh.</notification_summary>",
			},
		},
		"toolCount": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dirty), "<notification_summary>") {
		t.Fatal("fixture does not exercise the escaped marker")
	}
	cleaned := stripSubagentNotificationSummaries(dirty)
	var payload struct {
		AgentsStates map[string]struct {
			Message string `json:"message"`
		} `json:"agentsStates"`
	}
	if err := json.Unmarshal(cleaned, &payload); err != nil {
		t.Fatalf("cleaned payload is not valid JSON: %v", err)
	}
	if want := "Fixed the route. "; payload.AgentsStates["child-1"].Message != want {
		t.Fatalf("subagent message = %q, want %q", payload.AgentsStates["child-1"].Message, want)
	}
}

func TestStripSubagentNotificationSummariesPreservesSiblingNumbers(t *testing.T) {
	dirty := json.RawMessage(`{"agentsStates":{"child-1":{"message":"Report <notification_summary>private</notification_summary>","messageBytes":9007199254740993}}}`)
	cleaned := stripSubagentNotificationSummaries(dirty)
	var payload struct {
		AgentsStates map[string]map[string]json.RawMessage `json:"agentsStates"`
	}
	if err := json.Unmarshal(cleaned, &payload); err != nil {
		t.Fatalf("cleaned payload is not valid JSON: %v", err)
	}
	if got := string(payload.AgentsStates["child-1"]["messageBytes"]); got != "9007199254740993" {
		t.Fatalf("sibling number was not preserved byte-exactly: %s", got)
	}
}

func TestStripSubagentNotificationSummariesIgnoresPayloadsWithoutAgentStates(t *testing.T) {
	other := json.RawMessage(`{"detail":"<notification_summary>not a subagent payload</notification_summary>"}`)
	if got := stripSubagentNotificationSummaries(other); string(got) != string(other) {
		t.Fatalf("payload without agentsStates was rewritten: %s", got)
	}
}

func TestStripSubagentNotificationSummariesKeepsMalformedPayloadBytes(t *testing.T) {
	malformed := json.RawMessage(`{"agentsStates":{"child-1":{"message":"<notification_summary>"`) // truncated JSON
	if got := stripSubagentNotificationSummaries(malformed); string(got) != string(malformed) {
		t.Fatalf("malformed payload was rewritten: %s", got)
	}
}

func TestRelayStripsNotificationSummaryFromSubagentMessages(t *testing.T) {
	ctx := context.Background()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	chat, err := store.Create(ctx, servicechat.Meta{ID: "abcdef12", Provider: servicechat.ProviderCodex})
	if err != nil {
		t.Fatal(err)
	}
	provider := &collaborationPromptProvider{events: []agent.Event{
		{
			Type:     agent.EventCollaboration,
			ItemID:   "subagent:child-1",
			ItemKind: agent.ItemToolCall,
			ToolName: "Researcher",
			Status:   "completed",
			Data:     marshalSubagentThread(t),
		},
		{Type: agent.EventRunCompleted},
	}}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	service := New(store, nil, nil, runhub.New(store), registry)
	var live []ChatEvent
	emit := func(event ChatEvent) {
		live = append(live, event)
		if _, err := store.AppendEvent(ctx, chat.ID, event); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.runPrompt(ctx, chat.ID, "fix routes", emit, emit); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(ctx, chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	var persisted []ChatEvent
	for _, event := range events {
		if event.Type == "collaboration" {
			persisted = append(persisted, event)
		}
	}
	assertCleanCollaboration(t, persisted)
	assertCleanCollaboration(t, live)
}

// marshalSubagentThread builds the dirty collaboration payload the way the
// codexharness does, with json.Marshal escaping the marker.
func marshalSubagentThread(t *testing.T) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"type":              "subagentThread",
		"receiverThreadIds": []string{"child-1"},
		"agentsStates": map[string]any{
			"child-1": map[string]any{
				"status":  "completed",
				"message": "Fixed the route. <notification_summary>Chat links survive refresh.</notification_summary>",
			},
		},
		"toolCount":       1,
		"failedToolCount": 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertCleanCollaboration(t *testing.T, events []ChatEvent) {
	t.Helper()
	var found int
	for _, event := range events {
		if event.Type != "collaboration" {
			continue
		}
		found++
		if strings.Contains(string(event.Data), "notification_summary") {
			t.Fatalf("private marker reached chat history: %s", event.Data)
		}
		var payload struct {
			AgentsStates map[string]struct {
				Message string `json:"message"`
			} `json:"agentsStates"`
		}
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("collaboration data is not valid JSON: %v", err)
		}
		if want := "Fixed the route. "; payload.AgentsStates["child-1"].Message != want {
			t.Fatalf("subagent message = %q, want %q", payload.AgentsStates["child-1"].Message, want)
		}
	}
	if found == 0 {
		t.Fatal("no collaboration event was emitted")
	}
}
