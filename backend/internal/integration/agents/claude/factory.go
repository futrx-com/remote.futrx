package claude

import (
	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

// NewFactory returns Claude's complete module definition. The profile and
// descriptor are immutable policy; runtime and auth state are created afresh
// each time the catalog builds an application runtime.
func NewFactory() (agentmodule.Factory, error) {
	profile := Profile()
	profile.Credentials.HostDir = claudeHomeDir()
	profile.Credentials.Files[0].HostPath = claudeGlobalConfigPath()
	profile.Credentials.Files[1].HostPath = claudeCredentialPath()
	return agentmodule.NewFactory(agentmodule.Descriptor{
		ID:                  agent.ProviderClaude,
		Label:               "Claude",
		ExecutionScopes:     []agentmodule.ExecutionScope{agentmodule.ScopeHost, agentmodule.ScopeProject},
		Auth:                agentmodule.AuthManagedCode,
		AuthInstructions:    "Starts `claude auth login --claudeai` on the host. Sign in with your Anthropic subscription; credentials are shared with project containers.",
		SatisfiesAccessGate: true,
		LegacySkillRoots: []string{
			"/root/.claude/skills",
		},
		Features: agentmodule.Features{
			Sessions:              agentmodule.SessionSupport{Resume: true, Fork: true},
			Skills:                agentmodule.SkillsSlashCommand,
			BrowserTools:          true,
			ScheduledTools:        true,
			StreamingPresentation: agentmodule.StreamingBlocks,
		},
	}, &profile, func(deps agentmodule.Dependencies, validatedProfile *provisioning.Profile) (agentmodule.Components, error) {
		auth, err := NewAuth(deps.Accounts)
		if err != nil {
			return agentmodule.Components{}, err
		}
		binding := agentauth.NewCodeBinding(agent.ProviderClaude, auth.code)
		// A nil *AccountService in the interface would read as available.
		if auth.accounts != nil {
			binding = binding.WithAccounts(auth.accounts)
		}
		return agentmodule.Components{
			Provider: newProvider(
				deps.ProjectPreparer,
				deps.CredentialCollector,
				*validatedProfile,
				deps.CredentialSyncTimeout,
				auth.accounts,
			),
			Auth: &binding,
		}, nil
	}, agentmodule.WithProjectPreparation(agentmodule.ProjectPreparationPolicy{
		BrowserAssets:     true,
		BrowserMCPRuntime: true,
	}))
}

var (
	_ agent.Provider             = (*Provider)(nil)
	_ agentmodule.FactoryBuilder = NewFactory
)
