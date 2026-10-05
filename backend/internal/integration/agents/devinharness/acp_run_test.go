package devinharness

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// TestRunACPSuccessfulTurn tests a complete ACP turn: initialize → session/new
// → session/prompt → session/update notifications → prompt response with
// stopReason end_turn.
func TestRunACPSuccessfulTurn(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentCapabilities":{"loadSession":true},"authMethods":[{"id":"devin-browser","name":"Log in with browser"}],"agentInfo":{"name":"affogato","title":"Devin Agent","version":"0.0.0-dev"}}}'
      ;;
    *'"method":"session/new"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":2,"result":{"sessionId":"sess-1"}}'
      ;;
    *'"method":"session/prompt"'*)
      printf '%s\n' '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Hello!"},"messageId":"msg-1"}}}'
      printf '%s\n' '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess-1","update":{"sessionUpdate":"tool_call","toolCallId":"tc-1","title":"Bash","status":"inProgress","rawInput":{"command":"echo hi"}}}}'
      printf '%s\n' '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess-1","update":{"sessionUpdate":"tool_call_update","toolCallId":"tc-1","status":"completed","content":{"type":"text","text":"hi"}}}}'
      printf '%s\n' '{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}'
      ;;
  esac
done`

	var events []agent.Event
	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{
			Provider:       agent.ProviderDevin,
			ConversationID: "chat-1",
			Prompt:         "say hello",
		},
		"Devin",
		func(event agent.Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(events) < 4 {
		t.Fatalf("expected at least 4 events, got %d: %#v", len(events), events)
	}

	// Event 0: session updated
	if events[0].Type != agent.EventSessionUpdated || events[0].SessionID != "sess-1" {
		t.Fatalf("session event = %#v", events[0])
	}

	// Find the assistant text delta
	var foundAssistant, foundToolStarted, foundToolCompleted, foundCompletion bool
	for _, event := range events {
		switch event.Type {
		case agent.EventAssistantTextDelta:
			foundAssistant = true
			if event.Text != "Hello!" {
				t.Fatalf("assistant text = %q", event.Text)
			}
		case agent.EventToolStarted:
			foundToolStarted = true
			if event.ToolName != "Bash" || event.ItemID != "tc-1" {
				t.Fatalf("tool started = %#v", event)
			}
		case agent.EventToolCompleted:
			foundToolCompleted = true
			if event.Output != "hi" {
				t.Fatalf("tool output = %q", event.Output)
			}
		case agent.EventRunCompleted:
			foundCompletion = true
		}
	}
	if !foundAssistant {
		t.Fatal("missing assistant text delta event")
	}
	if !foundToolStarted {
		t.Fatal("missing tool started event")
	}
	if !foundToolCompleted {
		t.Fatal("missing tool completed event")
	}
	if !foundCompletion {
		t.Fatal("missing run completed event")
	}

	// Last event should be the completion
	if events[len(events)-1].Type != agent.EventRunCompleted {
		t.Fatalf("last event = %#v, want run completed", events[len(events)-1])
	}
}

// TestRunACPResume tests that session/resume is sent when ResumeID is set.
func TestRunACPResume(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[{"id":"devin-browser","name":"Log in with browser"}],"agentInfo":{"name":"affogato","title":"Devin Agent","version":"0.0.0-dev"}}}'
      ;;
    *'"method":"session/resume"'*)
      case "$line" in *'"sessionId":"existing-sess"'*) ;; *) exit 1 ;; esac
      printf '%s\n' '{"jsonrpc":"2.0","id":2,"result":{"sessionId":"existing-sess"}}'
      ;;
    *'"method":"session/prompt"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}'
      ;;
  esac
done`

	var events []agent.Event
	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{
			Provider:       agent.ProviderDevin,
			ConversationID: "chat-1",
			Prompt:         "continue",
			ResumeID:       "existing-sess",
		},
		"Devin",
		func(event agent.Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}

	// No session updated event when resuming the same session.
	for _, event := range events {
		if event.Type == agent.EventSessionUpdated {
			t.Fatalf("should not emit session updated when resuming same session: %#v", event)
		}
	}
	// Should still get a completion event.
	foundCompletion := false
	for _, event := range events {
		if event.Type == agent.EventRunCompleted {
			foundCompletion = true
		}
	}
	if !foundCompletion {
		t.Fatal("missing run completed event")
	}
}

// TestRunACPCancellation tests that context cancellation sends session/cancel
// and the prompt response with stopReason "cancelled" produces EventRunInterrupted.
func TestRunACPCancellation(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[{"id":"devin-browser","name":"Log in with browser"}],"agentInfo":{"name":"affogato","title":"Devin Agent","version":"0.0.0-dev"}}}'
      ;;
    *'"method":"session/new"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":2,"result":{"sessionId":"sess-cancel"}}'
      ;;
    *'"method":"session/prompt"'*)
      # Send a notification, then wait for the cancel notification
      printf '%s\n' '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess-cancel","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"working..."},"messageId":"msg-1"}}}'
      # Wait for cancel
      ;;
    *'"method":"session/cancel"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":3,"result":{"stopReason":"cancelled"}}'
      ;;
  esac
done`

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay to let the prompt be sent.
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	var events []agent.Event
	err := Run(
		ctx,
		exec.Command("sh", "-c", script),
		agent.RunRequest{
			Provider:       agent.ProviderDevin,
			ConversationID: "chat-1",
			Prompt:         "long task",
		},
		"Devin",
		func(event agent.Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}

	foundInterrupted := false
	for _, event := range events {
		if event.Type == agent.EventRunInterrupted {
			foundInterrupted = true
		}
		if event.Type == agent.EventRunCompleted {
			t.Fatal("should not get run completed on cancellation")
		}
	}
	if !foundInterrupted {
		t.Fatal("missing run interrupted event")
	}
}

// TestRunACPRefusal tests that stopReason "refusal" produces EventRunFailed.
func TestRunACPRefusal(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[{"id":"devin-browser","name":"Log in with browser"}],"agentInfo":{"name":"affogato","title":"Devin Agent","version":"0.0.0-dev"}}}'
      ;;
    *'"method":"session/new"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":2,"result":{"sessionId":"sess-refuse"}}'
      ;;
    *'"method":"session/prompt"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":3,"result":{"stopReason":"refusal"}}'
      ;;
  esac
done`

	var events []agent.Event
	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{
			Provider:       agent.ProviderDevin,
			ConversationID: "chat-1",
			Prompt:         "do something bad",
		},
		"Devin",
		func(event agent.Event) { events = append(events, event) },
	)
	if err == nil {
		t.Fatal("expected error for refusal, got nil")
	}

	foundFailed := false
	for _, event := range events {
		if event.Type == agent.EventRunFailed {
			foundFailed = true
			if event.Message != "agent refused to continue" {
				t.Fatalf("message = %q", event.Message)
			}
		}
	}
	if !foundFailed {
		t.Fatal("missing run failed event")
	}
}

// TestRunACPUnknownNotificationHandling tests that unknown notifications are
// forwarded as EventProviderNative without breaking the run.
func TestRunACPUnknownNotificationHandling(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[{"id":"devin-browser","name":"Log in with browser"}],"agentInfo":{"name":"affogato","title":"Devin Agent","version":"0.0.0-dev"}}}'
      ;;
    *'"method":"session/new"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":2,"result":{"sessionId":"sess-unk"}}'
      ;;
    *'"method":"session/prompt"'*)
      printf '%s\n' '{"jsonrpc":"2.0","method":"_cognition.ai/output","params":{"message":"MCP log"}}'
      printf '%s\n' '{"jsonrpc":"2.0","method":"_cognition.ai/customEvent","params":{"data":true}}'
      printf '%s\n' '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess-unk","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"response"},"messageId":"msg-1"}}}'
      printf '%s\n' '{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}'
      ;;
  esac
done`

	var events []agent.Event
	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{
			Provider:       agent.ProviderDevin,
			ConversationID: "chat-1",
			Prompt:         "hello",
		},
		"Devin",
		func(event agent.Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}

	// _cognition.ai/output should be filtered (no event).
	// _cognition.ai/customEvent should be native.
	// agent_message_chunk should be assistant delta.
	foundNativeCustom := false
	foundAssistant := false
	foundCompletion := false
	for _, event := range events {
		switch event.Type {
		case agent.EventProviderNative:
			if event.Native != nil && event.Native.Method == "_cognition.ai/customEvent" {
				foundNativeCustom = true
			}
		case agent.EventAssistantTextDelta:
			if event.Text == "response" {
				foundAssistant = true
			}
		case agent.EventRunCompleted:
			foundCompletion = true
		}
	}
	if !foundNativeCustom {
		t.Fatal("missing native event for _cognition.ai/customEvent")
	}
	if !foundAssistant {
		t.Fatal("missing assistant text delta event")
	}
	if !foundCompletion {
		t.Fatal("missing run completed event")
	}
}

// TestRunACPInitializeError tests that an initialize error is propagated.
func TestRunACPInitializeError(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"server not ready"}}'
      ;;
  esac
done`

	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{
			Provider:       agent.ProviderDevin,
			ConversationID: "chat-1",
			Prompt:         "hello",
		},
		"Devin",
		func(event agent.Event) {},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "server not ready") {
		t.Fatalf("error = %q, want it to contain 'server not ready'", err.Error())
	}
}

// TestRunACPSessionNewError tests that a session/new error is propagated.
func TestRunACPSessionNewError(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[{"id":"devin-browser","name":"Log in with browser"}],"agentInfo":{"name":"affogato","title":"Devin Agent","version":"0.0.0-dev"}}}'
      ;;
    *'"method":"session/new"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":2,"error":{"code":-1,"message":"Invalid params: missing field mcpServers"}}'
      ;;
  esac
done`

	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{
			Provider:       agent.ProviderDevin,
			ConversationID: "chat-1",
			Prompt:         "hello",
		},
		"Devin",
		func(event agent.Event) {},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mcpServers") {
		t.Fatalf("error = %q, want it to contain 'mcpServers'", err.Error())
	}
}

// TestRunACPMaxTokens tests that stopReason "max_tokens" produces EventRunCompleted.
func TestRunACPMaxTokens(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[{"id":"devin-browser","name":"Log in with browser"}],"agentInfo":{"name":"affogato","title":"Devin Agent","version":"0.0.0-dev"}}}'
      ;;
    *'"method":"session/new"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":2,"result":{"sessionId":"sess-max"}}'
      ;;
    *'"method":"session/prompt"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":3,"result":{"stopReason":"max_tokens"}}'
      ;;
  esac
done`

	var events []agent.Event
	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{
			Provider:       agent.ProviderDevin,
			ConversationID: "chat-1",
			Prompt:         "generate a lot",
		},
		"Devin",
		func(event agent.Event) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}

	foundCompletion := false
	for _, event := range events {
		if event.Type == agent.EventRunCompleted {
			foundCompletion = true
		}
	}
	if !foundCompletion {
		t.Fatal("missing run completed event for max_tokens")
	}
}

// TestRunACPNoAuthMethods tests that an initialize response without
// authMethods is treated as an error.
func TestRunACPNoAuthMethods(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[],"agentInfo":{"name":"affogato","title":"Devin Agent","version":"0.0.0-dev"}}}'
      ;;
  esac
done`

	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{
			Provider:       agent.ProviderDevin,
			ConversationID: "chat-1",
			Prompt:         "hello",
		},
		"Devin",
		func(event agent.Event) {},
	)
	if err == nil {
		t.Fatal("expected error for empty authMethods, got nil")
	}
	if !strings.Contains(err.Error(), "no auth methods") {
		t.Fatalf("error = %q, want it to contain 'no auth methods'", err.Error())
	}
}

// TestRunACPSessionMissing tests that a session/resume error for a missing
// session returns agent.ErrSessionNotFound.
func TestRunACPSessionMissing(t *testing.T) {
	script := `
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[{"id":"devin-browser","name":"Log in with browser"}],"agentInfo":{"name":"affogato","title":"Devin Agent","version":"0.0.0-dev"}}}'
      ;;
    *'"method":"session/resume"'*)
      printf '%s\n' '{"jsonrpc":"2.0","id":2,"error":{"code":-1,"message":"session not found: no such session"}}'
      ;;
  esac
done`

	err := Run(
		context.Background(),
		exec.Command("sh", "-c", script),
		agent.RunRequest{
			Provider:       agent.ProviderDevin,
			ConversationID: "chat-1",
			Prompt:         "continue",
			ResumeID:       "missing-sess",
		},
		"Devin",
		func(event agent.Event) {},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), agent.ErrSessionNotFound.Error()) {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), agent.ErrSessionNotFound.Error())
	}
}
