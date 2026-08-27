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

	if input.Chat != nil {
		if input.Chat.Provider != nil {
			provider := normalizeChatProvider(*input.Chat.Provider)
			if !s.validProvider(provider) {
				return Settings{}, ErrInvalidChatProvider
			}
			settings.Chat.Provider = provider
		}
		if input.Chat.Model != nil {
			settings.Chat.Model = strings.TrimSpace(*input.Chat.Model)
		}
		if input.Chat.Mode != nil {
			mode := normalizeChatMode(*input.Chat.Mode)
			if !ValidChatMode(mode) {
				return Settings{}, ErrInvalidChatMode
			}
			settings.Chat.Mode = mode
		}
		if input.Chat.ReasoningEffort != nil {
			effort := normalizeReasoningEffort(*input.Chat.ReasoningEffort)
			if !ValidReasoningEffort(effort) {
				return Settings{}, ErrInvalidReasoningEffort
			}
			settings.Chat.ReasoningEffort = effort
		}
		if input.Chat.ServiceTier != nil {
			tier := normalizeServiceTier(*input.Chat.ServiceTier)
			if !ValidServiceTier(tier) {
				return Settings{}, ErrInvalidServiceTier
			}
			settings.Chat.ServiceTier = tier
		}
	}

	settings.UpdatedAt = time.Now().UnixMilli()
	return s.repo.Save(ctx, key, settings)
}

func (s *Service) normalize(settings Settings) Settings {
	defaults := s.defaultSettings()
	if !ValidTheme(settings.Appearance.Theme) {
		settings.Appearance.Theme = defaults.Appearance.Theme
	}
	settings.Chat.Provider = normalizeChatProvider(settings.Chat.Provider)
	if !s.validProvider(settings.Chat.Provider) {
		settings.Chat.Provider = defaults.Chat.Provider
	}
	settings.Chat.Model = strings.TrimSpace(settings.Chat.Model)
	settings.Chat.Mode = normalizeChatMode(settings.Chat.Mode)
	if !ValidChatMode(settings.Chat.Mode) {
		settings.Chat.Mode = defaults.Chat.Mode
	}
	settings.Chat.ReasoningEffort = normalizeReasoningEffort(settings.Chat.ReasoningEffort)
	if !ValidReasoningEffort(settings.Chat.ReasoningEffort) {
		settings.Chat.ReasoningEffort = defaults.Chat.ReasoningEffort
	}
	settings.Chat.ServiceTier = normalizeServiceTier(settings.Chat.ServiceTier)
	if !ValidServiceTier(settings.Chat.ServiceTier) {
		settings.Chat.ServiceTier = defaults.Chat.ServiceTier
	}
	return settings
}

func (s *Service) defaultSettings() Settings {
	settings := DefaultSettings()
	if providers, ok := s.providers.(defaultProviderCatalog); ok {
		if provider := providers.DefaultProvider(agentmodule.ScopeHost); provider != "" {
			settings.Chat.Provider = provider
		}
	}
	return settings
}

func normalizeChatProvider(provider ChatProvider) ChatProvider {
	return agent.NormalizeProviderID(string(provider))
}

func (s *Service) validProvider(provider ChatProvider) bool {
	if !ValidChatProvider(provider) {
		return false
	}
	return s.providers == nil ||
		(s.providers.HasProvider(string(provider)) && s.providers.SupportsScope(string(provider), agentmodule.ScopeHost))
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
