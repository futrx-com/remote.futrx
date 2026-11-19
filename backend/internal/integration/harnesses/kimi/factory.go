package kimi

import (
	"github.com/futrx-com/remote.futrx.com/internal/harness"
	"github.com/futrx-com/remote.futrx.com/internal/harness/provisioning"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

// NewFactory returns Kimi's complete module definition, including its runtime,
// authentication flow, feature declarations, and provisioning profile.
func NewFactory() (agentmodule.Factory, error) {
	profile := Profile()
	return agentmodule.NewFactory(agentmodule.Descriptor{
		ID:                  harness.ProviderKimi,
		Label:               "Kimi",
		ExecutionScopes:     []agentmodule.ExecutionScope{agentmodule.ScopeHost, agentmodule.ScopeProject},
		Auth:                agentmodule.AuthManagedDevice,
		AuthInstructions:    "Starts `kimi login` on the host. Sign in with your Kimi Code subscription using the displayed device code.",
		SatisfiesAccessGate: true,
		Features: agentmodule.Features{
			Sessions:              agentmodule.SessionSupport{Resume: true},
			Skills:                agentmodule.SkillsInstructions,
			ApplicationTools:      true,
			StreamingPresentation: agentmodule.StreamingBlocks,
		},
	}, &profile, func(deps agentmodule.Dependencies, validatedProfile *provisioning.Profile) (agentmodule.Components, error) {
		binding := agentauth.NewDeviceBinding(harness.ProviderKimi, NewAuth())
		return agentmodule.Components{
			Provider: newProvider(
				deps.ProjectPreparer,
				deps.CredentialCollector,
				*validatedProfile,
				deps.CredentialSyncTimeout,
				deps.AccountAuthorizer,
			),
			Auth: &binding,
		}, nil
	}, agentmodule.WithProjectPreparation(agentmodule.ProjectPreparationPolicy{
		BrowserAssets: true,
	}))
}

var (
	_ harness.Provider           = (*Provider)(nil)
	_ agentmodule.FactoryBuilder = NewFactory
)
