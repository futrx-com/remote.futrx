package codex

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
	err := runAppServer(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{ConversationID: "chat-1", Mode: agent.RunModePlan, Prompt: "plan it"},
		func(event agent.Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Type != agent.EventSessionUpdated || events[0].SessionID != "thread-new" {
		t.Fatalf("session event = %#v", events[0])
	}
	if events[1].Type != agent.EventAssistantTextDelta || events[1].Text != "Native plan" {
		t.Fatalf("plan event = %#v", events[1])
	}
	if events[2].Type != agent.EventRunCompleted {
		t.Fatalf("completion event = %#v", events[2])
	}
	usage, ok := agent.ParseUsage(events[2].Usage)
	if !ok || usage.Model != "gpt-test" || usage.InputTokens != 7 ||
		usage.CacheReadTokens != 3 || usage.OutputTokens != 4 {
		t.Fatalf("completion usage = %#v (ok=%t)", usage, ok)
	}
}

func TestRunAppServerMapsMissingResumeThread(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"id":2'*) printf '%s\n' '{"id":2,"error":{"code":-1,"message":"thread not found"}}'; exit 0 ;;
  esac
done`

	err := runAppServer(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{ResumeID: "missing", Prompt: "continue"},
		func(agent.Event) {},
	)
	if !errors.Is(err, agent.ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestAnswerAppServerRequestDeclinesMutationInPlan(t *testing.T) {
	var encoded strings.Builder
	emitCalls := 0
	err := newAppServerRequestHandler(
		agent.RunRequest{Mode: agent.RunModePlan},
		func(agent.Event) { emitCalls++ },
		func(message any) error {
			data, marshalErr := json.Marshal(message)
			if marshalErr == nil {
				encoded.Write(data)
			}
			return marshalErr
		},
	).Answer(appServerEnvelope{
		ID:     []byte("42"),
		Method: "item/fileChange/requestApproval",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encoded.String(), `"decision":"decline"`) {
		t.Fatalf("response = %s", encoded.String())
	}
	if emitCalls != 0 {
		t.Fatalf("unexpected events = %d", emitCalls)
	}
}
