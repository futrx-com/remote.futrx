package antigravity

import (
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

// Parser translates line-oriented Antigravity output for focused parser tests.
// agy print mode emits unstructured text, so every line is an assistant delta.
// The provider's run loop streams raw chunks instead, preserving blank lines
// that line scanners drop.
type Parser struct {
	req harness.RunRequest
}

func NewParser(req harness.RunRequest) *Parser {
	if req.Provider == "" {
		req.Provider = harness.ProviderAntigravity
	}
	return &Parser{req: req}
}

func (p *Parser) ParseLine(line []byte) ([]harness.Event, error) {
	if len(line) == 0 {
		return nil, nil
	}
	return []harness.Event{{
		T:              time.Now().UnixMilli(),
		Type:           harness.EventAssistantTextDelta,
		Provider:       harness.ProviderAntigravity,
		ConversationID: p.req.ConversationID,
		ItemKind:       harness.ItemMessage,
		Text:           string(line) + "\n",
	}}, nil
}
