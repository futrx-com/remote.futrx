package skills

import "errors"

var ErrInvalidProvider = errors.New("invalid skill provider")

type Provider string

const (
	ProviderClaude      Provider = "claude"
	ProviderCodex       Provider = "codex"
	ProviderKimi        Provider = "kimi"
	ProviderAntigravity Provider = "antigravity"
)

type Skill struct {
	Name        string   `json:"name"`
	Command     string   `json:"command,omitempty"`
	Description string   `json:"description,omitempty"`
	Provider    Provider `json:"provider"`
	Source      string   `json:"source,omitempty"`
	// Scope is set to "global" for entries served by the platform-wide
	// library; project and host entries leave it empty.
	Scope string `json:"scope,omitempty"`
	// AlwaysOn marks a global skill an admin pinned into every new chat.
	AlwaysOn bool `json:"alwaysOn,omitempty"`
	// Shadowed marks a global skill hidden behind a same-named project skill.
	Shadowed bool `json:"shadowed,omitempty"`
	// ReadOnly marks entries a project member cannot edit from the project.
	ReadOnly bool `json:"readOnly,omitempty"`
}
