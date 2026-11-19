package antigravity

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
)

func TestStreamPromptUsesStdinAndPreservesLargeInput(t *testing.T) {
	p := &Provider{}
	prompt := "first" + strings.Repeat("λ", 100000) + "last"
	req := harness.RunRequest{Prompt: prompt, TrackInputAcceptance: true, Cwd: t.TempDir()}
	args := p.args(req)
	for _, arg := range args {
		if strings.Contains(arg, prompt) || arg == "--print" {
			t.Fatalf("prompt leaked into argv: %#v", args)
		}
	}
	cmd, _, err := p.buildCmd(context.Background(), req, args, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(cmd.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Event   string
		Message struct{ Content string }
	}
	if err = json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	if input.Event != "user" || input.Message.Content != prompt {
		t.Fatal("stdin prompt changed")
	}
}

func TestStreamParserTracksAcceptanceToolsAndDoesNotDuplicateResult(t *testing.T) {
	parser := newStreamParser(harness.RunRequest{TrackInputAcceptance: true})
	lines := []string{
		`{"event":"init","conversation_id":"native"}`,
		`{"event":"step_update","step_update":{"conversation_id":"native","step_index":0,"state":"DONE","step_type":"user_input"}}`,
		`{"event":"step_update","step_update":{"conversation_id":"native","step_index":1,"state":"DONE","step_type":"tool","tool_name":"run_command","tool_info":{"name":"run_command","parameters":{"CommandLine":"tests"},"output":"passed"}}}`,
		`{"event":"step_update","step_update":{"conversation_id":"native","step_index":2,"state":"ACTIVE","step_type":"agent_response","text_delta":"hello"}}`,
		`{"event":"step_update","step_update":{"conversation_id":"native","step_index":2,"state":"DONE","step_type":"agent_response","text_delta":"\n"}}`,
		`{"event":"result","result":{"conversation_id":"native","status":"SUCCESS","response":"hello\n"}}`,
	}
	text := ""
	accepted, started, finished, completed := 0, 0, 0, 0
	for index, line := range lines {
		events, err := parser.ParseLine([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Native == nil || len(event.Raw) == 0 {
				t.Fatal("native event lost")
			}
			switch event.Type {
			case harness.EventInputAccepted:
				accepted++
				if index == 0 || event.SessionID != "native" {
					t.Fatal("init accepted prompt")
				}
			case harness.EventToolStarted:
				started++
				if event.ItemID != "native:1" || string(event.Input) != `{"CommandLine":"tests"}` {
					t.Fatal(event)
				}
			case harness.EventToolCompleted:
				finished++
				if event.ItemID != "native:1" || event.Output != "passed" {
					t.Fatal(event)
				}
			case harness.EventAssistantTextDelta:
				text += event.Text
			case harness.EventRunCompleted:
				completed++
			}
		}
	}
	if text != "hello\n" || accepted != 1 || started != 1 || finished != 1 || completed != 1 {
		t.Fatalf("text=%q accepted=%d tools=%d/%d completed=%d", text, accepted, started, finished, completed)
	}
}
