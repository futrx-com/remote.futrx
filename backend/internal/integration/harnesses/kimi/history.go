package kimi

import (
	"fmt"
	"github.com/futrx-com/remote.futrx.com/internal/model/conversation"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

// HistoryInput translates Remote's portable history into context accepted by
// this harness's existing prompt interface. It does not rewrite native state.
func (p *Provider) HistoryInput(history conversation.History, prompt string) (string, string, error) {
	input, method, err := harness.PromptWithHistory(history, prompt)
	if err != nil {
		return "", "", err
	}
	// Kimi's installed prompt interface has no stdin/file-input equivalent.
	// Keep a conservative margin below Linux's single-argument limit rather
	// than silently truncating history or failing later with opaque E2BIG.
	if len(input) > maxPromptArgumentBytes {
		return "", "", fmt.Errorf("Kimi prompt requires %d bytes, exceeding the current command transport limit of %d bytes; no conversation history was truncated or delivered", len(input), maxPromptArgumentBytes)
	}
	return input, method, nil
}

var _ harness.HistoryInputAdapter = (*Provider)(nil)

const maxPromptArgumentBytes = 120 * 1024
