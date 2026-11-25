package codexharness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

func TestRunAppServerForksAtVerifiedNativeCheckpoint(t *testing.T) {
	for _, provider := range []harness.ProviderID{harness.ProviderCodex, harness.ProviderMiniMax} {
		t.Run(string(provider), func(t *testing.T) {
			events, requests, err := runCheckpointServer(t, provider,
				`{"id":2,"result":{"thread":{"id":"fork-thread"},"model":"gpt-test"}}`,
				`{"id":5,"result":{"data":[{"id":"retained-turn","status":"completed"}]}}`,
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(requests) != 5 || requests[2].Method != "thread/fork" || requests[3].Method != "thread/turns/list" || requests[4].Method != "turn/start" {
				t.Fatalf("request sequence = %#v", requests)
			}
			var fork appServerThreadParams
			if err := json.Unmarshal(requests[2].Params, &fork); err != nil {
				t.Fatal(err)
			}
			if fork.ThreadID != "source-thread" || fork.LastTurnID != "retained-turn" || !fork.ExcludeTurns || !fork.DeferGoalContinuation {
				t.Fatalf("fork request = %#v", fork)
			}
			var verify struct {
				ThreadID      string `json:"threadId"`
				Limit         int    `json:"limit"`
				SortDirection string `json:"sortDirection"`
				ItemsView     string `json:"itemsView"`
			}
			if err := json.Unmarshal(requests[3].Params, &verify); err != nil {
				t.Fatal(err)
			}
			if verify.ThreadID != "fork-thread" || verify.Limit != 1 || verify.SortDirection != "desc" || verify.ItemsView != "notLoaded" {
				t.Fatalf("verification request = %#v", verify)
			}
			var turn appServerTurnParams
			if err := json.Unmarshal(requests[4].Params, &turn); err != nil {
				t.Fatal(err)
			}
			if turn.ThreadID != "fork-thread" || len(turn.Input) != 1 || turn.Input[0].Text != "new prompt" {
				t.Fatalf("new turn = %#v", turn)
			}
			if len(events) != 3 || events[0].Type != harness.EventSessionUpdated || events[0].SessionID != "fork-thread" ||
				events[1].Type != harness.EventInputAccepted || events[2].Type != harness.EventRunCompleted || events[2].NativeCheckpoint != "new-turn" {
				t.Fatalf("events = %#v", events)
			}
			for _, event := range events {
				if event.Provider != provider {
					t.Fatalf("provider = %q, want %q", event.Provider, provider)
				}
			}
		})
	}
}

func TestRunAppServerRejectsUnsafeCheckpointBeforeSendingPrompt(t *testing.T) {
	for _, test := range []struct {
		name     string
		thread   string
		response string
	}{
		{name: "later turns retained", response: `{"id":5,"result":{"data":[{"id":"discarded-turn","status":"completed"}]}}`},
		{name: "missing turn", response: `{"id":5,"result":{"data":[]}}`},
		{name: "incomplete turn", response: `{"id":5,"result":{"data":[{"id":"retained-turn","status":"inProgress"}]}}`},
		{name: "interrupted turn", response: `{"id":5,"result":{"data":[{"id":"retained-turn","status":"interrupted"}]}}`},
		{name: "unknown verification method", response: `{"id":5,"error":{"code":-32601,"message":"Unknown method"}}`},
		{name: "malformed verification", response: `{"id":5,"result":{"data":"invalid"}}`},
		{name: "extra turns", response: `{"id":5,"result":{"data":[{"id":"retained-turn","status":"completed"},{"id":"discarded-turn","status":"completed"}]}}`},
		{name: "source reused", thread: `{"id":2,"result":{"thread":{"id":"source-thread"},"model":"gpt-test"}}`},
		{name: "missing native session", thread: `{"id":2,"error":{"code":-1,"message":"thread not found"}}`},
		{name: "missing native checkpoint", thread: `{"id":2,"error":{"code":-1,"message":"lastTurnId not found"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			thread := test.thread
			if thread == "" {
				thread = `{"id":2,"result":{"thread":{"id":"fork-thread"},"model":"gpt-test"}}`
			}
			events, requests, err := runCheckpointServer(t, harness.ProviderCodex, thread, test.response)
			if !errors.Is(err, harness.ErrNativeCheckpointUnavailable) {
				t.Fatalf("error = %v, want unavailable native checkpoint", err)
			}
			for _, request := range requests {
				if request.Method == "turn/start" || request.Method == "thread/resume" || request.Method == "thread/rollback" || request.Method == "thread/revert" {
					t.Fatalf("unsafe native request: %#v", request)
				}
			}
			if len(events) != 0 {
				t.Fatalf("unverified fork emitted events: %#v", events)
			}
		})
	}
}

func TestRunAppServerCheckpointRequiresForkAndSource(t *testing.T) {
	for _, request := range []harness.RunRequest{
		{NativeCheckpoint: "turn"},
		{NativeCheckpoint: "turn", ResumeID: "thread"},
		{NativeCheckpoint: "turn", Fork: true},
	} {
		command := exec.Command("false")
		err := Run(context.Background(), command, request, "Codex", nil)
		if !errors.Is(err, harness.ErrNativeCheckpointUnavailable) || command.Process != nil {
			t.Fatalf("invalid request launched provider: request=%#v, error=%v", request, err)
		}
	}
}

func TestAppServerOnlySuccessfulTurnsExposeNativeCheckpoint(t *testing.T) {
	for _, status := range []string{"completed", "interrupted", "failed", "unknown"} {
		t.Run(status, func(t *testing.T) {
			parser := newAppServerEventParser(harness.RunRequest{Provider: harness.ProviderCodex}, "Codex")
			raw := mustJSON(map[string]any{"threadId": "thread", "turn": map[string]string{"id": "turn", "status": status}})
			events := parser.ParseNotification("turn/completed", raw)
			if len(events) != 1 {
				t.Fatalf("events = %#v", events)
			}
			want := ""
			if status == "completed" {
				want = "turn"
			}
			if events[0].NativeCheckpoint != want {
				t.Fatalf("checkpoint = %q, want %q", events[0].NativeCheckpoint, want)
			}
		})
	}
}

func runCheckpointServer(t *testing.T, provider harness.ProviderID, threadResponse, checkpointResponse string) ([]harness.Event, []appServerEnvelope, error) {
	t.Helper()
	requestPath := filepath.Join(t.TempDir(), "requests.jsonl")
	script := `
while IFS= read -r line; do
  printf '%s\n' "$line" >> "$1"
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"id":2'*) printf '%s\n' "$2" ;;
    *'"id":5'*) printf '%s\n' "$3" ;;
    *'"id":3'*)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"new-turn","status":"inProgress"}}}'
      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"fork-thread","turn":{"id":"new-turn","status":"completed"}}}'
      exit 0
      ;;
  esac
done`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var events []harness.Event
	err := Run(ctx, exec.Command("sh", "-c", script, "checkpoint-server", requestPath, threadResponse, checkpointResponse), harness.RunRequest{
		Provider: provider, ConversationID: "chat", ResumeID: "source-thread", Fork: true,
		NativeCheckpoint: "retained-turn", Prompt: "new prompt", TrackInputAcceptance: true,
	}, "Codex", func(event harness.Event) { events = append(events, event) })
	raw, readErr := os.ReadFile(requestPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var requests []appServerEnvelope
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var envelope appServerEnvelope
		if decodeErr := json.Unmarshal([]byte(line), &envelope); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		requests = append(requests, envelope)
	}
	return events, requests, err
}
