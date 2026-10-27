package kimicodex

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

func (p *Provider) Capabilities(ctx context.Context, _ agent.CapabilityRequest) (agent.Capabilities, error) {
	caps := agent.Capabilities{
		Provider: agent.ProviderKimiCodex, Label: configconstants.KimiCodexLabel,
		Source: agent.CapabilitySourceFallback,
		Models: agent.WithAutoModel(nil, configconstants.KimiCodexAutoModelLabel),
		Modes:  agent.ProviderModes(true), DefaultMode: agent.RunModeDefault,
	}
	key, err := p.apiKey()
	if err != nil {
		caps.Warning = "Add a Kimi Code API key in Settings → Agents to discover models"
		return caps, err
	}
	if p.models == nil {
		return caps, ErrKimiCodexModelDiscoveryUnavailable
	}
	models, err := p.models.Models(ctx, key)
	if err != nil {
		caps.Warning = "Kimi models could not be read; check your API key and Kimi plan"
		return caps, err
	}
	var options []agent.ModelCapability
	for index, model := range models {
		reasoning := []agent.CapabilityOption{agent.AutoOption()}
		for _, effort := range model.Efforts {
			label := map[agent.ReasoningEffort]string{"low": "Low", "high": "High", "max": "Max"}[effort]
			reasoning = append(reasoning, agent.CapabilityOption{Value: string(effort), Label: label})
		}
		options = append(options, agent.ModelCapability{
			ID: model.ID, Label: model.Label, Description: "Kimi Code",
			ProviderDefault: index == 0, ReasoningEfforts: reasoning,
			DefaultReasoningEffort: string(model.DefaultEffort),
		})
	}
	caps.Source = agent.CapabilitySourceLive
	caps.Models = agent.WithAutoModel(options, configconstants.KimiCodexAutoModelLabel)
	return caps, nil
}
