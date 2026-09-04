package usersettings

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

type Service struct {
	repo      Repository
	providers ProviderCatalog
}

type ProviderCatalog interface {
	HasProvider(provider string) bool
	SupportsScope(provider string, scope agentmodule.ExecutionScope) bool
}

type defaultProviderCatalog interface {
	DefaultProvider(scope agentmodule.ExecutionScope) agent.ProviderID
}

type Option func(*Service)

func WithProviderCatalog(providers ProviderCatalog) Option {
	return func(service *Service) {
		service.providers = providers
	}
}

func New(repo Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func KeyFromSession(email, sub string) (Key, error) {
	sub = strings.TrimSpace(sub)
	if sub != "" {
		return Key("sub:" + sub), nil
	}

	email = strings.ToLower(strings.TrimSpace(email))
	if email != "" {
		return Key("email:" + email), nil
	}

	return "", ErrInvalidIdentity
}

func (s *Service) Get(ctx context.Context, key Key) (Settings, error) {
	if strings.TrimSpace(string(key)) == "" {
		return Settings{}, ErrInvalidIdentity
	}

	settings, err := s.repo.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return s.defaultSettings(), nil
		}
		return Settings{}, err
	}
	return s.normalize(settings), nil
}

func (s *Service) Update(ctx context.Context, key Key, input UpdateInput) (Settings, error) {
	settings, err := s.Get(ctx, key)
	if err != nil {
		return Settings{}, err
	}

	if input.Appearance != nil && input.Appearance.Theme != nil {
		theme := Theme(strings.TrimSpace(string(*input.Appearance.Theme)))
		if !ValidTheme(theme) {
			return Settings{}, ErrInvalidTheme
		}
		settings.Appearance.Theme = theme
	}

	if err := s.applyChatUpdate(&settings.Chat, input.Chat, agentmodule.ScopeHost); err != nil {
		return Settings{}, err
	}
	if err := s.applyChatUpdate(&settings.ProjectChat, input.ProjectChat, agentmodule.ScopeProject); err != nil {
		return Settings{}, err
	}

	settings.UpdatedAt = time.Now().UnixMilli()
	return s.repo.Save(ctx, key, settings)
}

func (s *Service) normalize(settings Settings) Settings {
	defaults := s.defaultSettings()
	if !ValidTheme(settings.Appearance.Theme) {
		settings.Appearance.Theme = defaults.Appearance.Theme
	}

	// Settings written before projectChat existed used chat for both scopes.
	// Preserve that preference when its provider can also run in a project.
	projectChat := settings.ProjectChat
	if normalizeChatProvider(projectChat.Provider) == "" &&
		s.validProviderForScope(settings.Chat.Provider, agentmodule.ScopeProject) {
		projectChat = settings.Chat
	}
	settings.Chat = s.normalizeChat(settings.Chat, defaults.Chat, agentmodule.ScopeHost)
	settings.ProjectChat = s.normalizeChat(projectChat, defaults.ProjectChat, agentmodule.ScopeProject)
	return settings
}

func (s *Service) defaultSettings() Settings {
	settings := DefaultSettings()
	if providers, ok := s.providers.(defaultProviderCatalog); ok {
		if provider := providers.DefaultProvider(agentmodule.ScopeHost); provider != "" {
			settings.Chat.Provider = provider
		}
		if provider := providers.DefaultProvider(agentmodule.ScopeProject); provider != "" {
			settings.ProjectChat.Provider = provider
		}
	}
	return settings
}

func (s *Service) applyChatUpdate(chat *Chat, update *ChatUpdate, scope agentmodule.ExecutionScope) error {
	if update == nil {
		return nil
	}
	if update.Provider != nil {
		provider := normalizeChatProvider(*update.Provider)
		if !s.validProviderForScope(provider, scope) {
			return ErrInvalidChatProvider
		}
		chat.Provider = provider
	}
	if update.Model != nil {
		chat.Model = strings.TrimSpace(*update.Model)
	}
	if update.Mode != nil {
		mode := normalizeChatMode(*update.Mode)
		if !ValidChatMode(mode) {
			return ErrInvalidChatMode
		}
		chat.Mode = mode
	}
	if update.ReasoningEffort != nil {
		effort := normalizeReasoningEffort(*update.ReasoningEffort)
		if !ValidReasoningEffort(effort) {
			return ErrInvalidReasoningEffort
		}
		chat.ReasoningEffort = effort
	}
	if update.ServiceTier != nil {
		tier := normalizeServiceTier(*update.ServiceTier)
		if !ValidServiceTier(tier) {
			return ErrInvalidServiceTier
		}
		chat.ServiceTier = tier
	}
	if update.ApprovalPolicy != nil {
		policy := ApprovalPolicy(strings.TrimSpace(string(*update.ApprovalPolicy)))
		if !ValidApprovalPolicy(policy) {
			return ErrInvalidApprovalPolicy
		}
		chat.ApprovalPolicy = policy
	}
	if update.SandboxPolicy != nil {
		policy := SandboxPolicy(strings.TrimSpace(string(*update.SandboxPolicy)))
		if !ValidSandboxPolicy(policy) {
			return ErrInvalidSandboxPolicy
		}
		chat.SandboxPolicy = policy
	}
	return nil
}

func (s *Service) normalizeChat(chat, defaults Chat, scope agentmodule.ExecutionScope) Chat {
	chat.Provider = normalizeChatProvider(chat.Provider)
	if !s.validProviderForScope(chat.Provider, scope) {
		chat.Provider = defaults.Provider
	}
	chat.Model = strings.TrimSpace(chat.Model)
	chat.Mode = normalizeChatMode(chat.Mode)
	if !ValidChatMode(chat.Mode) {
		chat.Mode = defaults.Mode
	}
	chat.ReasoningEffort = normalizeReasoningEffort(chat.ReasoningEffort)
	if !ValidReasoningEffort(chat.ReasoningEffort) {
		chat.ReasoningEffort = defaults.ReasoningEffort
	}
	chat.ServiceTier = normalizeServiceTier(chat.ServiceTier)
	if !ValidServiceTier(chat.ServiceTier) {
		chat.ServiceTier = defaults.ServiceTier
	}
	chat.ApprovalPolicy = ApprovalPolicy(strings.TrimSpace(string(chat.ApprovalPolicy)))
	if !ValidApprovalPolicy(chat.ApprovalPolicy) {
		chat.ApprovalPolicy = defaults.ApprovalPolicy
	}
	chat.SandboxPolicy = SandboxPolicy(strings.TrimSpace(string(chat.SandboxPolicy)))
	if !ValidSandboxPolicy(chat.SandboxPolicy) {
		chat.SandboxPolicy = defaults.SandboxPolicy
	}
	return chat
}

func normalizeChatProvider(provider ChatProvider) ChatProvider {
	return agent.NormalizeProviderID(string(provider))
}

func (s *Service) validProviderForScope(provider ChatProvider, scope agentmodule.ExecutionScope) bool {
	if !ValidChatProvider(provider) {
		return false
	}
	return s.providers == nil ||
		(s.providers.HasProvider(string(provider)) && s.providers.SupportsScope(string(provider), scope))
}

func normalizeChatMode(mode ChatMode) ChatMode {
	return ChatMode(strings.ToLower(strings.TrimSpace(string(mode))))
}

func normalizeReasoningEffort(effort ReasoningEffort) ReasoningEffort {
	return ReasoningEffort(strings.TrimSpace(string(effort)))
}

func normalizeServiceTier(tier ServiceTier) ServiceTier {
	return ServiceTier(strings.TrimSpace(string(tier)))
}
