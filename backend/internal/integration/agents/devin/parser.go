package devin

import (
	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// Parser is a no-op line parser. The ACP harness (devinharness.Run) emits
// agent.Event values directly from its JSON-RPC state machine — there is no
// line-oriented stdout output to parse. This parser exists only to satisfy the
// agent.LineParser contract for callers that request one via Provider.Parser().
type Parser struct {
	req agent.RunRequest
}

func NewParser(req agent.RunRequest) *Parser {
	if req.Provider == "" {
		req.Provider = agent.ProviderDevin
	}
	return &Parser{req: req}
}

// ParseLine always returns no events. The ACP harness handles all event
// translation internally.
func (p *Parser) ParseLine(line []byte) ([]agent.Event, error) {
	return nil, nil
}
