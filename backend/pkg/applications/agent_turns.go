package applications

import (
	"encoding/json"
	"errors"
)

var (
	ErrAgentAccess         = errors.New("application agent access denied")
	ErrAgentBusy           = errors.New("agent execution is busy")
	ErrAgentTurnNotFound   = errors.New("application agent turn not found")
	ErrAgentRequestChanged = errors.New("agent request ID was reused with different input")
	ErrAgentUnavailable    = errors.New("application agent runtime unavailable")
)

// AgentTurns is a host-owned capability bound to one installed application.
// Backend code supplies the owner captured from Request.Caller. Remote derives
// that owner's current authority and fences project installations to their project.
type AgentTurns interface {
	Start(AgentTurnRequest) (AgentTurn, error)
	Read(AgentTurnQuery) (AgentTurn, error)
	Forget(requestID string) error
}

// AgentTurnRequest identifies one logical operation. Retrying identical input
// with the same RequestID returns the accepted turn. Context is application-
// owned data made available to that turn's scoped application tools.
type AgentTurnRequest struct {
	RequestID  string          `json:"requestId"`
	ChatID     string          `json:"chatId"`
	OwnerEmail string          `json:"ownerEmail"`
	Prompt     string          `json:"prompt"`
	Context    json.RawMessage `json:"context,omitempty"`
}

type AgentTurnQuery struct {
	RequestID string `json:"requestId"`
	BeforeSeq int64  `json:"beforeSeq,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// AgentTurn reports execution state and a page of the existing chat transcript.
// Status is running, succeeded, failed, or interrupted. Interrupted work may be
// started again with the same request ID after a host restart (at least once).
type AgentTurn struct {
	RequestID  string            `json:"requestId"`
	ChatID     string            `json:"chatId"`
	TurnID     string            `json:"turnId,omitempty"`
	Status     string            `json:"status"`
	Output     string            `json:"output,omitempty"`
	Error      string            `json:"error,omitempty"`
	Events     []json.RawMessage `json:"events,omitempty"`
	NextBefore int64             `json:"nextBefore,omitempty"`
	HasMore    bool              `json:"hasMore,omitempty"`
}

// AgentContext is stamped by Remote on capability-authenticated application
// calls. Applications own the interpretation of Context and Background.
type AgentContext struct {
	ChatID     string          `json:"chatId"`
	Background bool            `json:"background,omitempty"`
	RequestID  string          `json:"requestId,omitempty"`
	Context    json.RawMessage `json:"context,omitempty"`
}
