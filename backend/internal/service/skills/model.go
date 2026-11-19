package skills

import (
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
)

var ErrInvalidProvider = errors.New("invalid skill provider")

type Provider = harness.ProviderID

const (
	ProviderClaude      = harness.ProviderClaude
	ProviderCodex       = harness.ProviderCodex
	ProviderKimi        = harness.ProviderKimi
	ProviderAntigravity = harness.ProviderAntigravity
)

type Skill struct {
	Name        string   `json:"name"`
	Command     string   `json:"command,omitempty"`
	Description string   `json:"description,omitempty"`
	Provider    Provider `json:"provider"`
	Source      string   `json:"source,omitempty"`
}
