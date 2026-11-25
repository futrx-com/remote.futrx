package claude

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

func TestNativeForkKeepsSourceAccountHomeAcrossTargetResumes(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	saved := harnessauth.RunCredential{AccountID: "account", Credential: testClaudeCredential("saved", "uuid-account", "account@example.test")}
	source, err := newAccountRun(harness.RunRequest{ProjectID: "project", ConversationID: "source"}, saved)
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []harness.RunRequest{
		{ProjectID: "project", ConversationID: "target", NativeConversationID: "source", ResumeID: "source-session", Fork: true, NativeCheckpoint: "assistant-uuid"},
		{ProjectID: "project", ConversationID: "target", NativeConversationID: "source", ResumeID: "fork-session"},
	} {
		run, err := newAccountRun(request, saved)
		if err != nil {
			t.Fatal(err)
		}
		if run.hostHome != source.hostHome || run.containerHome != source.containerHome {
			t.Fatalf("native session home changed: source=%#v target=%#v", source, run)
		}
		if request.ConversationID != "target" {
			t.Fatal("native scope must not change event/provisioning chat identity")
		}
	}
}

func TestNativeCheckpointUsesExactForkBoundary(t *testing.T) {
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	args := provider.args(harness.RunRequest{ResumeID: "source-session", Fork: true, NativeCheckpoint: "assistant-uuid"})
	for _, argument := range []string{"--resume", "source-session", "--fork-session", "--resume-session-at=assistant-uuid"} {
		if !slices.Contains(args, argument) {
			t.Fatalf("missing %q in %q", argument, args)
		}
	}
	for _, request := range []harness.RunRequest{
		{NativeCheckpoint: "assistant-uuid"},
		{ResumeID: "source-session", NativeCheckpoint: "assistant-uuid"},
		{Fork: true, NativeCheckpoint: "assistant-uuid"},
	} {
		if err := provider.Run(context.Background(), request, nil); err == nil {
			t.Fatalf("invalid boundary request must fail before launching: %#v", request)
		}
	}
}

func TestParserCompletesWithLastMainAssistantCheckpoint(t *testing.T) {
	parser := NewParser(harness.RunRequest{TrackInputAcceptance: true})
	for _, line := range []string{
		`{"type":"system","subtype":"init","session_id":"fork-session"}`,
		`{"type":"assistant","uuid":"tool-call-message","message":{"content":[{"type":"tool_use","id":"tool-1","name":"Read","input":{}}]}}`,
		`{"type":"user","uuid":"tool-result-message","message":{"content":[{"type":"tool_result","tool_use_id":"tool-1","content":"ok"}]}}`,
		`{"type":"assistant","uuid":"final-answer","message":{"content":[{"type":"text","text":"done"}],"stop_reason":"end_turn"}}`,
		`{"type":"assistant","uuid":"subagent-answer","parent_tool_use_id":"agent-tool","message":{"content":[{"type":"text","text":"child done"}]}}`,
		`{"type":"user","uuid":"subagent-tool-result","parent_tool_use_id":"agent-tool","message":{"content":[]}}`,
		`{"type":"stream_event","uuid":"stream-wrapper","event":{"type":"message_stop"}}`,
	} {
		if _, err := parser.ParseLine([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	events, err := parser.ParseLine([]byte(`{"type":"result","subtype":"success","uuid":"result-wrapper","session_id":"fork-session"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != harness.EventRunCompleted || events[0].NativeCheckpoint != "final-answer" {
		t.Fatalf("incorrect completed checkpoint: %#v", events)
	}
}

func TestParserDoesNotOfferAnUnfinishedNativeCheckpoint(t *testing.T) {
	for _, test := range []struct {
		name   string
		lines  []string
		result string
	}{
		{"no assistant", nil, `{"type":"result","subtype":"success"}`},
		{"tool-only message", []string{`{"type":"assistant","uuid":"tool-message","message":{"content":[{"type":"tool_use","id":"tool-1"}]}}`}, `{"type":"result","subtype":"success"}`},
		{"text with pending tool", []string{`{"type":"assistant","uuid":"tool-message","message":{"content":[{"type":"text","text":"working"},{"type":"tool_use","id":"tool-1"}]}}`}, `{"type":"result","subtype":"success"}`},
		{"tool after earlier answer", []string{`{"type":"assistant","uuid":"old-answer","message":{"content":[{"type":"text","text":"working"}]}}`, `{"type":"assistant","uuid":"tool-message","message":{"content":[{"type":"tool_use","id":"tool-1"}]}}`}, `{"type":"result","subtype":"success"}`},
		{"input after earlier answer", []string{`{"type":"assistant","uuid":"old-answer","message":{"content":[{"type":"text","text":"done"}]}}`, `{"type":"user","uuid":"new-input","message":{"content":[]}}`}, `{"type":"result","subtype":"success"}`},
		{"failed run", []string{`{"type":"assistant","uuid":"answer","message":{"content":[{"type":"text","text":"partial"}]}}`}, `{"type":"result","is_error":true,"result":"failed"}`},
		{"assistant error", []string{`{"type":"assistant","uuid":"error-answer","error":"rate_limit","message":{"content":[{"type":"text","text":"limited"}]}}`}, `{"type":"result","subtype":"success"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			parser := NewParser(harness.RunRequest{})
			for _, line := range append(test.lines, test.result) {
				events, err := parser.ParseLine([]byte(line))
				if err != nil {
					t.Fatal(err)
				}
				for _, event := range events {
					if event.NativeCheckpoint != "" {
						t.Fatalf("unsafe boundary on event: %#v", event)
					}
				}
			}
		})
	}
}

func TestNativeCheckpointFailureCanRecoverOnlyBeforeInputStarts(t *testing.T) {
	for _, test := range []struct {
		name    string
		body    string
		recover bool
	}{
		{"missing boundary", `printf '%s\n' 'No message found with message.uuid of: missing-message' >&2; exit 1`, true},
		{"unsupported boundary", `printf '%s\n' "error: unknown option '--resume-session-at=missing-message'" >&2; exit 1`, true},
		{"different missing message", `printf '%s\n' 'No message found with message.uuid of: other-message' >&2; exit 1`, false},
		{"auth error", `printf '%s\n' 'Authentication failed' >&2; exit 1`, false},
		{"error after execution", `printf '%s\n' '{"type":"assistant","uuid":"accepted-message","session_id":"fork-session","message":{"content":[{"type":"text","text":"started"}]}}'; printf '%s\n' 'No message found with message.uuid of: missing-message' >&2; exit 1`, false},
		{"missing session after execution", `printf '%s\n' '{"type":"assistant","uuid":"accepted-message","session_id":"fork-session","message":{"content":[{"type":"text","text":"started"}]}}'; printf '%s\n' 'No conversation found with session ID: source-session' >&2; exit 1`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			installFakeClaude(t, test.body)
			provider := newTestProvider(nil, provisioning.ContainerDependencies{})
			err := provider.Run(context.Background(), harness.RunRequest{Cwd: t.TempDir(), ResumeID: "source-session", Fork: true, NativeCheckpoint: "missing-message"}, nil)
			if err == nil || errors.Is(err, harness.ErrNativeCheckpointUnavailable) != test.recover {
				t.Fatalf("error = %v, recoverable = %v", err, test.recover)
			}
			if errors.Is(err, harness.ErrSessionNotFound) {
				t.Fatalf("unsafe missing-session recovery: %v", err)
			}
		})
	}
}
