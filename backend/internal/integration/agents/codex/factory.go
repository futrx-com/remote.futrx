package codex

import (
	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

// NewFactory returns Codex's complete module definition. Runtime and auth state
// are constructed together for each application runtime so the normalized
// warning observes the same auth instance as the binding.
func NewFactory() (agentmodule.Factory, error) {
	profile := Profile()
	return agentmodule.NewFactory(agentmodule.Descriptor{
		ID:                  agent.ProviderCodex,
		Label:               "Codex",
		Default:             true,
		ExecutionScopes:     []agentmodule.ExecutionScope{agentmodule.ScopeHost, agentmodule.ScopeProject},
		Auth:                agentmodule.AuthManagedDevice,
		AuthInstructions:    "Starts `codex login --device-auth` on the host. Sign in with ChatGPT so Codex uses subscription limits instead of API-key billing.",
		SatisfiesAccessGate: true,
		LegacySkillRoots: []string{
			"/root/.codex/skills",
		},
		Features: agentmodule.Features{
			Sessions:       agentmodule.SessionSupport{Resume: true, Fork: true},
			Skills:         agentmodule.SkillsDollarMention,
			BrowserTools:   true,
			ScheduledTools: true,
		},
	}, &profile, func(deps agentmodule.Dependencies, validatedProfile *provisioning.Profile) (agentmodule.Components, error) {
		auth := NewAuth()
		binding := agentauth.NewDeviceBinding(agent.ProviderCodex, auth).WithWarning(func() string {
			if auth.Status().UsesAPIKey {
				return "Codex is logged in with an API key. Sign in with ChatGPT to use subscription limits."
			}
			return ""
		})
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
		CLIErrorOperation:  "codex CLI unavailable in container",
		BeforeCredentials:  validateSubscriptionCredentials,
		SkillLinksRequired: true,
		BrowserAssets:      true,
		BrowserMCPRuntime:  true,
	}))
}

var (
	_ agent.Provider             = (*Provider)(nil)
	_ agentmodule.FactoryBuilder = NewFactory
)
