package kimi

import (
	"github.com/futrx-com/remote.futrx.com/internal/model/conversation"
	"strings"
	"testing"
)

func TestHistoryInputRejectsUnsupportedArgvSizeWithoutTruncation(t *testing.T) {
	p := &Provider{}
	history := conversation.History{Records: []conversation.Record{{Kind: "assistant", Text: strings.Repeat("x", maxPromptArgumentBytes)}}}
	input, _, err := p.HistoryInput(history, "continue")
	if err == nil || input != "" || !strings.Contains(err.Error(), "no conversation history was truncated") {
		t.Fatalf("input=%q err=%v", input, err)
	}
}
