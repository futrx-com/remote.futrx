package codexharness

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestRunAppServerStreamsNativePlanTurn(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*)
      printf '%s\n' '{"id":1,"result":{}}'
      ;;
    *'"id":2'*)
      printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-new"},"model":"gpt-test"}}'
      ;;
    *'"id":3'*)
      case "$line" in
        *'"mode":"plan"'*) ;;
        *) printf '%s\n' '{"id":3,"error":{"code":-1,"message":"missing native plan mode"}}'; exit 0 ;;
      esac
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-1","status":"inProgress","items":[]}}}'
      printf '%s\n' '{"method":"item/plan/delta","params":{"threadId":"thread-new","turnId":"turn-1","itemId":"plan-1","delta":"Native plan"}}'
      printf '%s\n' '{"method":"thread/tokenUsage/updated","params":{"tokenUsage":{"last":{"inputTokens":10,"cachedInputTokens":3,"cacheWriteInputTokens":0,"outputTokens":4,"reasoningOutputTokens":2}}}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread-new","turn":{"id":"turn-1","status":"completed","items":[]}}}'
      exit 0
      ;;
  esac
done`

	var events []agent.Event
	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{Provider: agent.ProviderMiniMax, ConversationID: "chat-1", Mode: agent.RunModePlan, Prompt: "plan it"},
		"MiniMax",
		func(event agent.Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("events = %#v", events)
	}
	for _, event := range events {
		if event.Provider != agent.ProviderMiniMax {
			t.Fatalf("event provider = %q, want minimax: %#v", event.Provider, event)
		}
	}
	if events[0].Type != agent.EventSessionUpdated || events[0].SessionID != "thread-new" {
		t.Fatalf("session event = %#v", events[0])
	}
	if events[1].Type != agent.EventAssistantTextDelta || events[1].Text != "Native plan" {
		t.Fatalf("plan event = %#v", events[1])
	}
	if events[2].Type != agent.EventUsageUpdated {
		t.Fatalf("usage event = %#v", events[2])
	}
	if events[3].Type != agent.EventRunCompleted {
		t.Fatalf("completion event = %#v", events[3])
	}
	usage, ok := agent.ParseUsage(events[3].Usage)
	if !ok || usage.Model != "gpt-test" || usage.InputTokens != 7 ||
		usage.CacheReadTokens != 3 || usage.OutputTokens != 4 {
		t.Fatalf("completion usage = %#v (ok=%t)", usage, ok)
	}
}

func TestRunAppServerKeepsChildTurnsSeparateUntilParentCompletes(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"id":2'*) printf '%s\n' '{"id":2,"result":{"thread":{"id":"parent-thread"},"model":"gpt-test"}}' ;;
    *'"id":3'*)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"parent-turn","status":"inProgress","items":[]}}}'
      printf '%s\n' '{"method":"turn/started","params":{"threadId":"child-thread","turn":{"id":"child-turn","status":"inProgress","items":[]}}}'
      printf '%s\n' '{"method":"item/started","params":{"threadId":"child-thread","turnId":"child-turn","item":{"id":"child-tool","type":"commandExecution","status":"inProgress","command":"inspect"}}}'
      printf '%s\n' '{"method":"item/completed","params":{"threadId":"child-thread","turnId":"child-turn","item":{"id":"child-tool","type":"commandExecution","status":"completed","command":"inspect","aggregatedOutput":"done"}}}'
      printf '%s\n' '{"method":"item/completed","params":{"threadId":"child-thread","turnId":"child-turn","item":{"id":"child-message","type":"agentMessage","text":"child report"}}}'
      printf '%s\n' '{"method":"thread/tokenUsage/updated","params":{"threadId":"child-thread","tokenUsage":{"last":{"inputTokens":100,"cachedInputTokens":10,"outputTokens":20,"reasoningOutputTokens":0}}}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"child-thread","turn":{"id":"child-turn","status":"completed","items":[{"id":"child-message","type":"agentMessage","text":"child report"}]}}}'
      printf '%s\n' '{"method":"thread/status/changed","params":{"threadId":"child-thread","status":{"type":"idle"}}}'
      printf '%s\n' '{"method":"item/completed","params":{"threadId":"parent-thread","turnId":"parent-turn","item":{"id":"parent-message","type":"agentMessage","text":"parent synthesis"}}}'
      printf '%s\n' '{"method":"thread/tokenUsage/updated","params":{"threadId":"parent-thread","tokenUsage":{"last":{"inputTokens":12,"cachedInputTokens":2,"outputTokens":4,"reasoningOutputTokens":0}}}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"parent-thread","turn":{"id":"parent-turn","status":"completed","items":[{"id":"parent-message","type":"agentMessage","text":"parent synthesis"}]}}}'
      exit 0
      ;;
  esac
done`

	var events []agent.Event
	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{Provider: agent.ProviderCodex, ConversationID: "chat-1", Prompt: "delegate"},
		"Codex",
		func(event agent.Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}

	var assistantText strings.Builder
	var completions []agent.Event
	for _, event := range events {
		if event.Type == agent.EventAssistantTextDelta {
			assistantText.WriteString(event.Text)
		}
		if event.Type == agent.EventRunCompleted {
			completions = append(completions, event)
		}
	}
	if assistantText.String() != "parent synthesis" {
		t.Fatalf("main assistant text = %q, want only parent synthesis", assistantText.String())
	}
	if len(completions) != 1 || completions[0].Native == nil || completions[0].Native.ThreadID != "parent-thread" {
		t.Fatalf("run completions = %#v", completions)
	}
	usage, ok := agent.ParseUsage(completions[0].Usage)
	if !ok || usage.InputTokens != 10 || usage.CacheReadTokens != 2 || usage.OutputTokens != 4 {
		t.Fatalf("parent completion usage = %#v (ok=%t)", usage, ok)
	}

	collaborations := collaborationEvents(events)
	if len(collaborations) == 0 {
		t.Fatalf("no subagent activity emitted: %#v", events)
	}
	last := collaborations[len(collaborations)-1]
	if last.ItemID != "subagent:child-thread" || last.Status != "completed" {
		t.Fatalf("final subagent state = %#v", last)
	}
	var data map[string]any
	if err := json.Unmarshal(last.Data, &data); err != nil {
		t.Fatal(err)
	}
	states, _ := data["agentsStates"].(map[string]any)
	child, _ := states["child-thread"].(map[string]any)
	if child["message"] != "child report" || data["toolCount"] != float64(1) {
		t.Fatalf("subagent activity data = %#v", data)
	}
	tools, _ := data["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("subagent tools = %#v", data["tools"])
	}
	tool, _ := tools[0].(map[string]any)
	input, _ := tool["input"].(map[string]any)
	startedAt, started := tool["startedAt"].(float64)
	completedAt, completed := tool["completedAt"].(float64)
	duration, timed := tool["durationMs"].(float64)
	if tool["name"] != "Bash" || tool["status"] != "completed" ||
		input["command"] != "inspect" || tool["output"] != "done" ||
		!started || !completed || !timed || completedAt < startedAt || duration < 0 {
		t.Fatalf("subagent tool detail = %#v", tool)
	}
	if events[len(events)-1].Type != agent.EventRunCompleted {
		t.Fatalf("parent completion was not the final event: %#v", events)
	}
}

func TestRunAppServerDrainsLateCollaborationCompletion(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"id":2'*) printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-1"},"model":"gpt-test"}}' ;;
    *'"id":3'*)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-1","status":"inProgress","items":[]}}}'
      printf '%s\n' '{"method":"item/started","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"wait-1","type":"collabAgentToolCall","tool":"wait","status":"inProgress","receiverThreadIds":["child-1"]}}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}'
      printf '%s\n' '{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"wait-1","type":"collabAgentToolCall","tool":"wait","status":"completed","receiverThreadIds":["child-1"],"agentsStates":{"child-1":{"status":"completed","message":"done"}}}}}'
      exit 0
      ;;
  esac
done`

	var events []agent.Event
	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{Provider: agent.ProviderCodex, ConversationID: "chat-1", Prompt: "wait"},
		"Codex",
		func(event agent.Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}

	collaborations := collaborationEvents(events)
	if len(collaborations) != 2 {
		t.Fatalf("collaboration events = %#v", collaborations)
	}
	if collaborations[0].Status != "inProgress" || collaborations[1].Status != "completed" {
		t.Fatalf("collaboration statuses = %q, %q", collaborations[0].Status, collaborations[1].Status)
	}
	if collaborations[1].Native == nil || collaborations[1].Native.Method != "item/completed" {
		t.Fatalf("late completion was not preserved as native: %#v", collaborations[1])
	}
	if events[len(events)-1].Type != agent.EventRunCompleted {
		t.Fatalf("terminal event must follow the drained item completion: %#v", events)
	}
}

func TestRunAppServerResolvesUnfinishedCollaborationBeforeCompletion(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"id":2'*) printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-1"},"model":"gpt-test"}}' ;;
    *'"id":3'*)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-1","status":"inProgress","items":[]}}}'
      printf '%s\n' '{"method":"item/started","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"wait-1","type":"collabAgentToolCall","tool":"wait","status":"inProgress","receiverThreadIds":["child-1"]}}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}'
      exit 0
      ;;
  esac
done`

	var events []agent.Event
	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{Provider: agent.ProviderCodex, ConversationID: "chat-1", Prompt: "wait"},
		"Codex",
		func(event agent.Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}

	collaborations := collaborationEvents(events)
	if len(collaborations) != 2 {
		t.Fatalf("collaboration events = %#v", collaborations)
	}
	resolved := collaborations[1]
	if resolved.Status != "turnEnded" || resolved.Native != nil || len(resolved.Raw) != 0 {
		t.Fatalf("synthetic collaboration resolution = %#v", resolved)
	}
	var data map[string]any
	if err := json.Unmarshal(resolved.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data["remoteResolution"] != "missingItemCompletion" || data["status"] != "turnEnded" {
		t.Fatalf("synthetic resolution data = %#v", data)
	}
	if events[len(events)-1].Type != agent.EventRunCompleted {
		t.Fatalf("terminal event must follow collaboration reconciliation: %#v", events)
	}
}

func collaborationEvents(events []agent.Event) []agent.Event {
	var collaborations []agent.Event
	for _, event := range events {
		if event.Type == agent.EventCollaboration {
			collaborations = append(collaborations, event)
		}
	}
	return collaborations
}

func TestRunAppServerMapsMissingResumeThread(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"id":2'*) printf '%s\n' '{"id":2,"error":{"code":-1,"message":"thread not found"}}'; exit 0 ;;
  esac
done`

	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{Provider: agent.ProviderCodex, ResumeID: "missing", Prompt: "continue"},
		"Codex",
		func(agent.Event) {},
	)
	if !errors.Is(err, agent.ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestAppServerParserUsesRequestProvider(t *testing.T) {
	parser := newAppServerEventParser(agent.RunRequest{Provider: agent.ProviderCodex}, "Codex")
	events := parser.ParseNotification("turn/completed", json.RawMessage(`{"turn":{"status":"completed"}}`))
	if len(events) != 1 || events[0].Provider != agent.ProviderCodex {
		t.Fatalf("events = %#v", events)
	}
}

func TestAppServerRequestStaysPendingUntilExplicitResponse(t *testing.T) {
	var encoded strings.Builder
	var events []agent.Event
	handler := newAppServerRequestHandler(
		agent.RunRequest{Provider: agent.ProviderCodex, Mode: agent.RunModePlan},
		func(event agent.Event) { events = append(events, event) },
		func(message any) error {
			data, marshalErr := json.Marshal(message)
			if marshalErr == nil {
				encoded.Write(data)
			}
			return marshalErr
		},
	)
	err := handler.Handle(appServerEnvelope{
		ID:     []byte(`"request-42"`),
		Method: "item/fileChange/requestApproval",
		Params: []byte(`{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if encoded.Len() != 0 {
		t.Fatalf("request was answered before user input: %s", encoded.String())
	}
	if len(events) != 1 || events[0].Type != agent.EventInteractionRequest || events[0].InteractionID != `"request-42"` {
		t.Fatalf("events = %#v", events)
	}
	if err := handler.Respond(agent.InteractionResponse{
		ID: `"request-42"`, Result: []byte(`{"decision":"decline"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encoded.String(), `"id":"request-42"`) || !strings.Contains(encoded.String(), `"decision":"decline"`) {
		t.Fatalf("response = %s", encoded.String())
	}
	if len(events) != 2 || events[1].Type != agent.EventInteractionDone || events[1].Status != "denied" {
		t.Fatalf("events = %#v", events)
	}
}

func TestAppServerDoesNotPersistSecretInteractionAnswers(t *testing.T) {
	var events []agent.Event
	handler := newAppServerRequestHandler(
		agent.RunRequest{ConversationID: "chat-1"},
		func(event agent.Event) { events = append(events, event) },
		func(any) error { return nil },
	)
	if err := handler.Handle(appServerEnvelope{
		ID:     []byte("9"),
		Method: "item/tool/requestUserInput",
		Params: []byte(`{"questions":[{"id":"token","question":"Token?","isSecret":true,"options":[]}]}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := handler.Respond(agent.InteractionResponse{
		ID: "9", Result: []byte(`{"answers":{"token":{"answers":["super-secret-value"]}}}`),
	}); err != nil {
		t.Fatal(err)
	}
	persisted, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persisted), "super-secret-value") {
		t.Fatalf("secret leaked into events: %s", persisted)
	}
}

func TestRunAppServerUsesNativeTurnInterrupt(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"id":2'*) printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-1"},"model":"gpt-test"}}' ;;
    *'"id":3'*)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-1","status":"inProgress","items":[]}}}'
      printf '%s\n' '{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"inProgress","items":[]}}}'
      ;;
    *'"id":4'*)
      case "$line" in
        *'"method":"turn/interrupt"'*'"threadId":"thread-1"'*'"turnId":"turn-1"'*) ;;
        *) exit 7 ;;
      esac
      printf '%s\n' '{"id":4,"result":{}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"interrupted","items":[]}}}'
      exit 0
      ;;
  esac
done`

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var events []agent.Event
	err := Run(
		ctx,
		exec.Command("sh", "-c", script),
		agent.RunRequest{Provider: agent.ProviderCodex, ConversationID: "chat-1", Prompt: "work"},
		"Codex",
		func(event agent.Event) {
			events = append(events, event)
			if event.Type == agent.EventTurnStatus && event.Status == "inProgress" {
				cancel()
			}
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || events[len(events)-1].Type != agent.EventRunInterrupted {
		t.Fatalf("events = %#v", events)
	}
}
