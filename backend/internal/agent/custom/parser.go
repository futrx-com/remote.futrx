package custom

// Parser satisfies the provider contract for line-oriented callers. The
// custom provider's Run streams OpenAI chat-completion deltas directly as
// assistant text events, so this parser is only used by callers that drive a
// line-oriented run loop; it treats each line as an assistant text delta.

import (
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type Parser struct {
	req agent.RunRequest
}

func NewParser(req agent.RunRequest) *Parser {
	if req.Provider == "" {
		req.Provider = agent.ProviderCustom
	}
	return &Parser{req: req}
}

func (p *Parser) ParseLine(line []byte) ([]agent.Event, error) {
	if len(line) == 0 {
		return nil, nil
	}
	return []agent.Event{{
		T:              time.Now().UnixMilli(),
		Type:           agent.EventAssistantTextDelta,
		Provider:       agent.ProviderCustom,
		ConversationID: p.req.ConversationID,
		ItemKind:       agent.ItemMessage,
		Text:           string(line) + "\n",
	}}, nil
}
