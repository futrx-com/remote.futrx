package antigravity

import (
	"github.com/futrx-com/remote.futrx.com/internal/harness"
	"github.com/futrx-com/remote.futrx.com/internal/model/conversation"
)

// HistoryInput translates Remote's portable history into context accepted by
// this harness's existing prompt interface. It does not rewrite native state.
func (p *Provider) HistoryInput(history conversation.History, prompt string) (string, string, error) {
	return harness.PromptWithHistory(history, prompt)
}

var _ harness.HistoryInputAdapter = (*Provider)(nil)
