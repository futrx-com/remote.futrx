package kimicodex

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

// NewFactory returns Kimi's project-scoped module. The provider uses the
// Codex app-server harness with a separate home and Kimi Code Responses endpoint.
func NewFactory() (agentmodule.Factory, error) {
	profile := Profile()
	return agentmodule.NewFactory(agentmodule.Descriptor{
		ID:               agent.ProviderKimiCodex,
		Label:            configconstants.KimiCodexLabel,
		ExecutionScopes:  []agentmodule.ExecutionScope{agentmodule.ScopeProject},
		Auth:             agentmodule.AuthManagedAPIKey,
		AuthInstructions: configconstants.KimiCodexAuthInstructions,
		APIKeyAuth: &agentmodule.APIKeyAuth{
			CreateURL:       configconstants.KimiCodexAPIKeyCreateURL,
			CreateLabel:     configconstants.KimiCodexAPIKeyCreateLabel,
			CredentialLabel: configconstants.KimiCodexAPIKeyCredentialLabel,
		},
		Features: agentmodule.Features{
			Sessions:              agentmodule.SessionSupport{Resume: true, Fork: true},
			Skills:                agentmodule.SkillsDollarMention,
			BrowserTools:          true,
			ApplicationTools:      true,
			ExecutionPolicies:     true,
			StreamingPresentation: agentmodule.StreamingBlocks,
		},
	}, &profile, func(deps agentmodule.Dependencies, validatedProfile *provisioning.Profile) (agentmodule.Components, error) {
		models := newModelCatalogClient()
		apiKeys, err := agentauth.NewAPIKeyService(
			context.Background(),
			agent.ProviderKimiCodex,
			deps.APIKeys,
			models,
		)
		if err != nil {
			return agentmodule.Components{}, err
		}
		if err := apiKeys.EnableAccounts(context.Background(), deps.Accounts, configconstants.KimiCodexLabel); err != nil {
			return agentmodule.Components{}, err
		}
		binding := agentauth.NewAPIKeyBinding(agent.ProviderKimiCodex, apiKeys)
		return agentmodule.Components{
			Provider: newProvider(
				deps.ProjectPreparer,
				apiKeys,
				models,
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
