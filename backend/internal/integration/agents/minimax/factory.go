package minimax

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

// NewFactory returns MiniMax's project-scoped module. The provider uses the
// Codex app-server harness with a separate home and MiniMax Responses endpoint.
func NewFactory() (agentmodule.Factory, error) {
	profile := Profile()
	return agentmodule.NewFactory(agentmodule.Descriptor{
		ID:               agent.ProviderMiniMax,
		Label:            configconstants.MiniMaxLabel,
		ExecutionScopes:  []agentmodule.ExecutionScope{agentmodule.ScopeProject},
		Auth:             agentmodule.AuthManagedAPIKey,
		AuthInstructions: configconstants.MiniMaxAuthInstructions,
		APIKeyAuth: &agentmodule.APIKeyAuth{
			CreateURL:       configconstants.MiniMaxAPIKeyCreateURL,
			CreateLabel:     configconstants.MiniMaxAPIKeyCreateLabel,
			CredentialLabel: configconstants.MiniMaxAPIKeyCredentialLabel,
		},
		Features: agentmodule.Features{
			Sessions:       agentmodule.SessionSupport{Resume: true, Fork: true},
			Skills:         agentmodule.SkillsDollarMention,
			BrowserTools:   true,
			ScheduledTools: true,
		},
	}, &profile, func(deps agentmodule.Dependencies, validatedProfile *provisioning.Profile) (agentmodule.Components, error) {
		apiKeys, err := agentauth.NewAPIKeyService(
			context.Background(),
			agent.ProviderMiniMax,
			deps.APIKeys,
			newAPIKeyValidator(),
		)
		if err != nil {
			return agentmodule.Components{}, err
		}
		binding := agentauth.NewAPIKeyBinding(agent.ProviderMiniMax, apiKeys)
		return agentmodule.Components{
			Provider: newProvider(
				deps.ProjectPreparer,
				apiKeys,
				newModelCatalogClient(),
				deps.RuntimeAssets,
				validatedProfile.CLI.Binary,
			),
			Auth: &binding,
		}, nil
	}, agentmodule.WithProjectPreparation(agentmodule.ProjectPreparationPolicy{
		SkillLinksRequired: true,
		BrowserAssets:      true,
		BrowserMCPRuntime:  true,
	}))
}

var (
	_ agent.Provider             = (*Provider)(nil)
	_ agentmodule.FactoryBuilder = NewFactory
)
