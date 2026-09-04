package skills

import (
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

var ErrInvalidProvider = errors.New("invalid skill provider")

type Provider = agent.ProviderID

const (
	ProviderClaude      = agent.ProviderClaude
	ProviderCodex       = agent.ProviderCodex
	ProviderKimi        = agent.ProviderKimi
	ProviderAntigravity = agent.ProviderAntigravity
)

type Skill struct {
	Name        string   `json:"name"`
	Command     string   `json:"command,omitempty"`
	Description string   `json:"description,omitempty"`
	Provider    Provider `json:"provider"`
	Source      string   `json:"source,omitempty"`
}
