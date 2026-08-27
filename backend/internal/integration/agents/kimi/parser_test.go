package kimi

import (
	"encoding/json"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// Representative kimi-code stream-json output for a tool-using turn:
// assistant line(s), a tool result line, and a trailing meta resume-hint line.
func TestParserToolUsingTurn(t *testing.T) {
	p := NewParser(agent.RunRequest{ConversationID: "conv1"})
	lines := []string{
		`{"role":"assistant","content":"Let me check.","tool_calls":[{"type":"function","id":"tc_1","function":{"name":"Shell","arguments":"{\"command\":\"ls\"}"}}]}`,
		`{"role":"tool","tool_call_id":"tc_1","content":"file1.py\nfile2.py"}`,
		`{"role":"assistant","content":"There are two Python files."}`,
		`{"role":"meta","type":"session.resume_hint","session_id":"session_abc","command":"kimi -r session_abc","content":"To resume this session: kimi -r session_abc"}`,
	}

	var got []agent.Event
	for _, line := range lines {
		evs, err := p.ParseLine([]byte(line))
		if err != nil {
			t.Fatalf("ParseLine(%s): %v", line, err)
		}
		got = append(got, evs...)
	}

	want := []agent.EventType{
		agent.EventAssistantTextDelta, // "Let me check."
		agent.EventToolStarted,        // Shell tool_call
		agent.EventToolCompleted,      // tool result
		agent.EventAssistantTextDelta, // "There are two Python files."
		agent.EventSessionUpdated,     // session_abc
		agent.EventRunCompleted,       // meta line marks completion
	}
	if len(got) != len(want) {
		t.Fatalf("event count = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Type != w {
			t.Fatalf("event[%d].Type = %s, want %s", i, got[i].Type, w)
		}
	}

	if got[0].Text != "Let me check." {
		t.Errorf("assistant text = %q", got[0].Text)
	}
	if got[1].ToolName != "Shell" || got[1].ItemID != "tc_1" {
		t.Errorf("tool start = %q id=%q", got[1].ToolName, got[1].ItemID)
	}
	// arguments is a JSON-encoded string upstream; it must survive as valid JSON.
	var input map[string]string
	if err := json.Unmarshal(got[1].Input, &input); err != nil {
		t.Fatalf("tool input not valid JSON: %v (raw=%s)", err, got[1].Input)
	}
	if input["command"] != "ls" {
		t.Errorf("tool input command = %q", input["command"])
	}
	if got[2].ItemID != "tc_1" || got[2].Output != "file1.py\nfile2.py" {
		t.Errorf("tool result id=%q output=%q", got[2].ItemID, got[2].Output)
	}
	if got[4].SessionID != "session_abc" {
		t.Errorf("session id = %q", got[4].SessionID)
	}
}

// A tool-only assistant line (no content) must not emit an empty text event.
func TestParserToolOnlyAssistant(t *testing.T) {
	p := NewParser(agent.RunRequest{ConversationID: "c"})
	evs, err := p.ParseLine([]byte(`{"role":"assistant","tool_calls":[{"type":"function","id":"t1","function":{"name":"Read","arguments":"{}"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Type != agent.EventToolStarted {
		t.Fatalf("got %+v, want a single tool-started event", evs)
	}
}

// The session id must only be reported once even if a resume id was supplied.
func TestParserSuppressesKnownSession(t *testing.T) {
	p := NewParser(agent.RunRequest{ConversationID: "c", ResumeID: "session_known"})
	evs, err := p.ParseLine([]byte(`{"role":"meta","type":"session.resume_hint","session_id":"session_known"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Type != agent.EventRunCompleted {
		t.Fatalf("got %+v, want only run-completed (no duplicate session.updated)", evs)
	}
}
