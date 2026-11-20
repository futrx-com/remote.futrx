package claude

import (
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	harnessmodule "github.com/futrx-com/remote.futrx.com/internal/service/harness/module"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

// NewFactory returns Claude's complete module definition. The profile and
// descriptor are immutable policy; runtime and auth state are created afresh
// each time the catalog builds an application runtime.
func NewFactory() (harnessmodule.Factory, error) {
	profile := Profile()
	profile.Credentials.HostDir = claudeHomeDir()
	profile.Credentials.Files[0].HostPath = claudeGlobalConfigPath()
	profile.Credentials.Files[1].HostPath = claudeCredentialPath()
	return harnessmodule.NewFactory(harnessmodule.Descriptor{
		ID:                  harness.ProviderClaude,
		Label:               "Claude",
		ExecutionScopes:     []harnessmodule.ExecutionScope{harnessmodule.ScopeHost, harnessmodule.ScopeProject},
		Auth:                harnessmodule.AuthManagedCode,
		AuthInstructions:    "Starts `claude auth login --claudeai` on the host. Sign in with your Anthropic subscription; credentials are shared with project containers.",
		SatisfiesAccessGate: true,
		LegacySkillRoots: []string{
			"/root/.claude/skills",
		},
		Features: harnessmodule.Features{
			Sessions:              harnessmodule.SessionSupport{Resume: true, Fork: true},
			Skills:                harnessmodule.SkillsSlashCommand,
			BrowserTools:          true,
			ApplicationTools:      true,
			StreamingPresentation: harnessmodule.StreamingBlocks,
		},
	}, &profile, func(deps harnessmodule.Dependencies, validatedProfile *provisioning.Profile) (harnessmodule.Components, error) {
		auth, err := NewAuth(deps.Accounts)
		if err != nil {
			return harnessmodule.Components{}, err
		}
		binding := harnessauth.NewCodeBinding(harness.ProviderClaude, auth.code)
		// A nil *AccountService in the interface would read as available.
		if auth.accounts != nil {
			binding = binding.WithAccounts(auth.accounts)
		}
		return harnessmodule.Components{
			Provider: newProvider(
				deps.ProjectPreparer,
				deps.CredentialCollector,
				*validatedProfile,
				deps.CredentialSyncTimeout,
				auth.accounts,
			),
			Auth: &binding,
		}, nil
	}, harnessmodule.WithProjectPreparation(harnessmodule.ProjectPreparationPolicy{
		BrowserAssets:     true,
		BrowserMCPRuntime: true,
	}))
}

var (
	_ harness.Provider             = (*Provider)(nil)
	_ harnessmodule.FactoryBuilder = NewFactory
)
