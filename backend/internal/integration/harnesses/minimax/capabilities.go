package minimax

import (
	"context"
	"fmt"
	"strings"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/harness"
)

func (p *Provider) Capabilities(ctx context.Context, _ harness.CapabilityRequest) (harness.Capabilities, error) {
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

	models := make([]harness.ModelCapability, 0, len(modelIDs))
	for index, id := range modelIDs {
		reasoning, defaultReasoning := miniMaxReasoningCapabilities(id)
		models = append(models, harness.ModelCapability{
			ID:                     id,
			Label:                  id,
			Description:            configconstants.MiniMaxLabel,
			ProviderDefault:        index == 0,
			ReasoningEfforts:       reasoning,
			DefaultReasoningEffort: string(defaultReasoning),
		})
	}

	return harness.Capabilities{
		Provider:    harness.ProviderMiniMax,
		Label:       configconstants.MiniMaxLabel,
		Source:      harness.CapabilitySourceLive,
		Models:      harness.WithAutoModel(models, configconstants.MiniMaxAutoModelLabel),
		Modes:       harness.ProviderModes(true),
		DefaultMode: harness.RunModeDefault,
	}, nil
}

func fallbackCapabilities() harness.Capabilities {
	return harness.Capabilities{
		Provider:    harness.ProviderMiniMax,
		Label:       configconstants.MiniMaxLabel,
		Source:      harness.CapabilitySourceFallback,
		Models:      harness.WithAutoModel(nil, configconstants.MiniMaxAutoModelLabel),
		Modes:       harness.ProviderModes(true),
		DefaultMode: harness.RunModeDefault,
	}
}

func miniMaxReasoningCapabilities(model string) ([]harness.CapabilityOption, harness.ReasoningEffort) {
	if !miniMaxSupportsThinkingToggle(model) {
		return nil, configconstants.MiniMaxReasoningAdaptive
	}
	return []harness.CapabilityOption{
		harness.AutoOption(),
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

func miniMaxReasoningEffort(model string, effort harness.ReasoningEffort) harness.ReasoningEffort {
	if miniMaxSupportsThinkingToggle(model) &&
		strings.EqualFold(strings.TrimSpace(string(effort)), configconstants.MiniMaxReasoningDisabled) {
		return configconstants.MiniMaxReasoningDisabled
	}
	return configconstants.MiniMaxReasoningAdaptive
}

func miniMaxSupportsThinkingToggle(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "minimax-m3")
}
