package skills

import "errors"

var ErrInvalidProvider = errors.New("invalid skill provider")

type Provider string

const (
	ProviderClaude      Provider = "claude"
	ProviderCodex       Provider = "codex"
	ProviderKimi        Provider = "kimi"
	ProviderAntigravity Provider = "antigravity"
	ProviderCustom      Provider = "custom"
)

type Skill struct {
	Name        string   `json:"name"`
	Command     string   `json:"command,omitempty"`
	Description string   `json:"description,omitempty"`
	Provider    Provider `json:"provider"`
	Source      string   `json:"source,omitempty"`
}
