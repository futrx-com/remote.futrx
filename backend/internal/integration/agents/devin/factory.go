package devin

import (
	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

// NewFactory returns Devin's complete module definition, including its runtime,
// authentication flow, feature declarations, and provisioning profile.
func NewFactory() (agentmodule.Factory, error) {
	profile := Profile()
	return agentmodule.NewFactory(agentmodule.Descriptor{
		ID:                  agent.ProviderDevin,
		Label:               "Devin",
		ExecutionScopes:     []agentmodule.ExecutionScope{agentmodule.ScopeHost, agentmodule.ScopeProject},
		Auth:                agentmodule.AuthManagedCode,
		AuthInstructions:    "Run `devin auth login --force-manual-token-flow` on the host, open the displayed URL, sign in, and paste the returned code.",
		SatisfiesAccessGate: true,
		Features: agentmodule.Features{
			Sessions:       agentmodule.SessionSupport{Resume: true},
			Skills:         agentmodule.SkillsInstructions,
			ScheduledTools: true,
		},
	}, &profile, func(deps agentmodule.Dependencies, validatedProfile *provisioning.Profile) (agentmodule.Components, error) {
		binding := agentauth.NewCodeBinding(agent.ProviderDevin, NewAuth())
		return agentmodule.Components{
			Provider: newProvider(
				deps.ProjectPreparer,
				deps.CredentialCollector,
				*validatedProfile,
				deps.CredentialSyncTimeout,
			),
			Auth: &binding,
		}, nil
	}, agentmodule.WithProjectPreparation(agentmodule.ProjectPreparationPolicy{
		BrowserAssets: true,
	}))
}

var (
	_ agent.Provider             = (*Provider)(nil)
	_ agentmodule.FactoryBuilder = NewFactory
)
