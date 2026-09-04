package usersettings

import (
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

var (
	ErrNotFound               = errors.New("user settings not found")
	ErrInvalidIdentity        = errors.New("user settings identity is required")
	ErrInvalidTheme           = errors.New("invalid appearance theme")
	ErrInvalidChatProvider    = errors.New("invalid chat provider")
	ErrInvalidChatMode        = errors.New("invalid chat mode")
	ErrInvalidReasoningEffort = errors.New("invalid reasoning effort")
	ErrInvalidServiceTier     = errors.New("invalid service tier")
	ErrInvalidApprovalPolicy  = errors.New("invalid approval policy")
	ErrInvalidSandboxPolicy   = errors.New("invalid sandbox policy")
)

type Key string

const LocalAdminKey Key = "local-admin"

type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeDark   Theme = "dark"
	ThemeLight  Theme = "light"
)

type Settings struct {
	Appearance  Appearance `json:"appearance"`
	Chat        Chat       `json:"chat"`
	ProjectChat Chat       `json:"projectChat"`
	UpdatedAt   int64      `json:"updatedAt,omitempty"`
}

type Appearance struct {
	Theme Theme `json:"theme"`
}

type ChatProvider = agent.ProviderID

const (
	ChatProviderClaude      = agent.ProviderClaude
	ChatProviderCodex       = agent.ProviderCodex
	ChatProviderMiniMax     = agent.ProviderMiniMax
	ChatProviderKimi        = agent.ProviderKimi
	ChatProviderAntigravity = agent.ProviderAntigravity
)

type ChatMode string

const (
	ChatModeDefault ChatMode = "default"
	ChatModePlan    ChatMode = "plan"
)

type ReasoningEffort string

const (
	ReasoningEffortAuto    ReasoningEffort = ""
	ReasoningEffortNone    ReasoningEffort = "none"
	ReasoningEffortMinimal ReasoningEffort = "minimal"
	ReasoningEffortLow     ReasoningEffort = "low"
	ReasoningEffortMedium  ReasoningEffort = "medium"
	ReasoningEffortHigh    ReasoningEffort = "high"
	ReasoningEffortXHigh   ReasoningEffort = "xhigh"
	ReasoningEffortMax     ReasoningEffort = "max"
	ReasoningEffortUltra   ReasoningEffort = "ultra"
)

type ServiceTier string
type ApprovalPolicy string
type SandboxPolicy string

const (
	ServiceTierAuto     ServiceTier = ""
	ServiceTierDefault  ServiceTier = "default"
	ServiceTierPriority ServiceTier = "priority"
	ServiceTierFast     ServiceTier = "fast"
)

type Chat struct {
	Provider        ChatProvider    `json:"provider"`
	Model           string          `json:"model"`
	Mode            ChatMode        `json:"mode"`
	ReasoningEffort ReasoningEffort `json:"reasoningEffort"`
	ServiceTier     ServiceTier     `json:"serviceTier"`
	ApprovalPolicy  ApprovalPolicy  `json:"approvalPolicy"`
	SandboxPolicy   SandboxPolicy   `json:"sandboxPolicy"`
}

type UpdateInput struct {
	Appearance  *AppearanceUpdate `json:"appearance,omitempty"`
	Chat        *ChatUpdate       `json:"chat,omitempty"`
	ProjectChat *ChatUpdate       `json:"projectChat,omitempty"`
}

type AppearanceUpdate struct {
	Theme *Theme `json:"theme,omitempty"`
}

type ChatUpdate struct {
	Provider        *ChatProvider    `json:"provider,omitempty"`
	Model           *string          `json:"model,omitempty"`
	Mode            *ChatMode        `json:"mode,omitempty"`
	ReasoningEffort *ReasoningEffort `json:"reasoningEffort,omitempty"`
	ServiceTier     *ServiceTier     `json:"serviceTier,omitempty"`
	ApprovalPolicy  *ApprovalPolicy  `json:"approvalPolicy,omitempty"`
	SandboxPolicy   *SandboxPolicy   `json:"sandboxPolicy,omitempty"`
}

func DefaultSettings() Settings {
	chat := defaultChatSettings()
	return Settings{
		Appearance:  Appearance{Theme: ThemeSystem},
		Chat:        chat,
		ProjectChat: chat,
	}
}

func defaultChatSettings() Chat {
	return Chat{
		Provider:        ChatProviderCodex,
		Model:           "",
		Mode:            ChatModeDefault,
		ReasoningEffort: ReasoningEffortAuto,
		ServiceTier:     ServiceTierAuto,
		ApprovalPolicy:  ApprovalPolicy(configconstants.DefaultAgentApprovalPolicy),
		SandboxPolicy:   SandboxPolicy(configconstants.DefaultAgentSandboxPolicy),
	}
}

func ValidTheme(theme Theme) bool {
	switch theme {
	case ThemeSystem, ThemeDark, ThemeLight:
		return true
	default:
		return false
	}
}

func ValidChatProvider(provider ChatProvider) bool {
	return agent.ValidProviderID(provider)
}

func ValidChatMode(mode ChatMode) bool {
	switch mode {
	case ChatModeDefault, ChatModePlan:
		return true
	default:
		return false
	}
}

func ValidReasoningEffort(effort ReasoningEffort) bool {
	return agent.ValidPreferenceValue(string(effort))
}

func ValidServiceTier(tier ServiceTier) bool {
	return agent.ValidPreferenceValue(string(tier))
}

func ValidApprovalPolicy(policy ApprovalPolicy) bool {
	return agent.ValidApprovalPolicy(string(policy))
}

func ValidSandboxPolicy(policy SandboxPolicy) bool {
	return agent.ValidSandboxPolicy(string(policy))
}
