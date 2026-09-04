package minimax

import (
	"context"
	"fmt"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

func (p *Provider) Capabilities(ctx context.Context, _ agent.CapabilityRequest) (agent.Capabilities, error) {
	if p.apiKeys == nil || p.models == nil {
		caps := fallbackCapabilities()
		caps.Warning = "MiniMax models could not be read from the provider"
		return caps, ErrMiniMaxAPIKeyMissing
	}
	key, ok := p.apiKeys.APIKey()
	if !ok {
		caps := fallbackCapabilities()
		caps.Warning = "Add a MiniMax Token Plan subscription key to discover models"
		return caps, ErrMiniMaxAPIKeyMissing
	}
	modelIDs, err := p.models.Models(ctx, key)
	if err != nil {
		caps := fallbackCapabilities()
		caps.Warning = "MiniMax models could not be read from the provider"
		return caps, fmt.Errorf("minimax capability discovery: %w", err)
	}

	models := make([]agent.ModelCapability, 0, len(modelIDs))
	for index, id := range modelIDs {
		reasoning, defaultReasoning := miniMaxReasoningCapabilities(id)
		models = append(models, agent.ModelCapability{
			ID:                     id,
			Label:                  id,
			Description:            configconstants.MiniMaxLabel,
			ProviderDefault:        index == 0,
			ReasoningEfforts:       reasoning,
			DefaultReasoningEffort: string(defaultReasoning),
		})
	}

	return agent.Capabilities{
		Provider:    agent.ProviderMiniMax,
		Label:       configconstants.MiniMaxLabel,
		Source:      agent.CapabilitySourceLive,
		Models:      agent.WithAutoModel(models, configconstants.MiniMaxAutoModelLabel),
		Modes:       agent.ProviderModes(true),
		DefaultMode: agent.RunModeDefault,
	}, nil
}

func fallbackCapabilities() agent.Capabilities {
	return agent.Capabilities{
		Provider:    agent.ProviderMiniMax,
		Label:       configconstants.MiniMaxLabel,
		Source:      agent.CapabilitySourceFallback,
		Models:      agent.WithAutoModel(nil, configconstants.MiniMaxAutoModelLabel),
		Modes:       agent.ProviderModes(true),
		DefaultMode: agent.RunModeDefault,
	}
}

func miniMaxReasoningCapabilities(model string) ([]agent.CapabilityOption, agent.ReasoningEffort) {
	if !miniMaxSupportsThinkingToggle(model) {
		return nil, configconstants.MiniMaxReasoningAdaptive
	}
	return []agent.CapabilityOption{
		agent.AutoOption(),
		{
			Value:       configconstants.MiniMaxReasoningDisabled,
			Label:       configconstants.MiniMaxReasoningDisabledLabel,
			Description: configconstants.MiniMaxReasoningDisabledDescription,
		},
		{
			Value:       configconstants.MiniMaxReasoningAdaptive,
			Label:       configconstants.MiniMaxReasoningAdaptiveLabel,
			Description: configconstants.MiniMaxReasoningAdaptiveDescription,
		},
	}, configconstants.MiniMaxReasoningAdaptive
}

func miniMaxReasoningEffort(model string, effort agent.ReasoningEffort) agent.ReasoningEffort {
	if miniMaxSupportsThinkingToggle(model) &&
		strings.EqualFold(strings.TrimSpace(string(effort)), configconstants.MiniMaxReasoningDisabled) {
		return configconstants.MiniMaxReasoningDisabled
	}
	return configconstants.MiniMaxReasoningAdaptive
}

func miniMaxSupportsThinkingToggle(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "minimax-m3")
}
