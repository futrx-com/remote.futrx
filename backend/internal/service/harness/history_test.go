package harness

import (
	"github.com/futrx-com/remote.futrx.com/internal/model/conversation"
	"strings"
	"testing"
)

func TestPromptWithHistoryPreservesLargeHistoryAndProvenance(t *testing.T) {
	text := "first-record" + strings.Repeat("unicode-λ", 10000) + "last-record"
	history := conversation.History{BranchID: "branch", AfterSeq: 12, ThroughSeq: 20, Records: []conversation.Record{
		{Seq: 13, Kind: "assistant", Provider: "codex", Text: text},
		{Seq: 14, Kind: "tool_call", Provider: "codex", ToolCallID: "call-1", Name: "shell"},
		{Seq: 15, Kind: "tool_result", Provider: "codex", ToolCallID: "call-1", Output: "tests passed"},
		{Seq: 16, Kind: "attachment", ContentRef: "sha256:example"},
	}}
	input, method, err := PromptWithHistory(history, "review changes")
	if err != nil {
		t.Fatal(err)
	}
	if method != PromptContextDelivery {
		t.Fatal(method)
	}
	for _, want := range []string{text, `"provider":"codex"`, `"toolCallId":"call-1"`, "tests passed", "sha256:example", "Current user request:\nreview changes"} {
		if !strings.Contains(input, want) {
			t.Fatalf("input lost %q", want[:min(len(want), 50)])
		}
	}
}

func TestPromptWithHistoryDoesNotDuplicateCurrentMessage(t *testing.T) {
	input, _, err := PromptWithHistory(conversation.History{Records: []conversation.Record{{Kind: "user", Text: "previous"}}}, "current")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(input, "current") != 1 {
		t.Fatal(input)
	}
}
