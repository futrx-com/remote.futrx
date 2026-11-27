package minimax

import (
	"context"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	harnessmodule "github.com/futrx-com/remote.futrx.com/internal/service/harness/module"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

// NewFactory returns MiniMax's project-scoped module. The provider uses the
// Codex app-server harness with a separate home and MiniMax Responses endpoint.
func NewFactory() (harnessmodule.Factory, error) {
	profile := Profile()
	return harnessmodule.NewFactory(harnessmodule.Descriptor{
		ID:               harness.ProviderMiniMax,
		Label:            configconstants.MiniMaxLabel,
		ExecutionScopes:  []harnessmodule.ExecutionScope{harnessmodule.ScopeProject},
		Auth:             harnessmodule.AuthManagedAPIKey,
		AuthInstructions: configconstants.MiniMaxAuthInstructions,
		APIKeyAuth: &harnessmodule.APIKeyAuth{
			CreateURL:       configconstants.MiniMaxAPIKeyCreateURL,
			CreateLabel:     configconstants.MiniMaxAPIKeyCreateLabel,
			CredentialLabel: configconstants.MiniMaxAPIKeyCredentialLabel,
		},
		Features: harnessmodule.Features{
			Sessions:              harnessmodule.SessionSupport{Resume: true, Fork: true},
			Skills:                harnessmodule.SkillsDollarMention,
			BrowserTools:          true,
			ApplicationTools:      true,
			ExecutionPolicies:     true,
			StreamingPresentation: harnessmodule.StreamingBlocks,
		},
	}, &profile, func(deps harnessmodule.Dependencies, validatedProfile *provisioning.Profile) (harnessmodule.Components, error) {
		tokenPlans := newAPIKeyValidator()
		apiKeys, err := harnessauth.NewAPIKeyService(
			context.Background(),
			harness.ProviderMiniMax,
			deps.APIKeys,
			tokenPlans,
			deps.AccountAuthorizer,
		)
		if err != nil {
			return harnessmodule.Components{}, err
		}
		if err := apiKeys.EnableAccounts(context.Background(), deps.Accounts, configconstants.MiniMaxLabel); err != nil {
			return harnessmodule.Components{}, err
		}
		binding := harnessauth.NewAPIKeyBinding(harness.ProviderMiniMax, apiKeys)
		return harnessmodule.Components{
			Provider: newProvider(
				deps.ProjectPreparer,
				apiKeys,
				newModelCatalogClient(),
				deps.RuntimeAssets,
				validatedProfile.CLI.Binary,
				tokenPlans,
			),
			Auth: &binding,
		}, nil
	}, harnessmodule.WithProjectPreparation(harnessmodule.ProjectPreparationPolicy{
		SkillLinksRequired: true,
		BrowserAssets:      true,
		BrowserMCPRuntime:  true,
	}))
}

var (
	_ harness.Provider             = (*Provider)(nil)
	_ harnessmodule.FactoryBuilder = NewFactory
)
