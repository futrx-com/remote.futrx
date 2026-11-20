package codex

import (
	"path/filepath"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	harnessmodule "github.com/futrx-com/remote.futrx.com/internal/service/harness/module"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

// NewFactory returns Codex's complete module definition. Runtime and auth state
// are constructed together for each application runtime so the normalized
// warning observes the same auth instance as the binding.
func NewFactory() (harnessmodule.Factory, error) {
	profile := Profile()
	credentialPath := codexCredentialPath()
	profile.Credentials.HostDir = filepath.Dir(credentialPath)
	profile.Credentials.Files[0].HostPath = credentialPath
	return harnessmodule.NewFactory(harnessmodule.Descriptor{
		ID:                  harness.ProviderCodex,
		Label:               "Codex",
		Default:             true,
		ExecutionScopes:     []harnessmodule.ExecutionScope{harnessmodule.ScopeHost, harnessmodule.ScopeProject},
		Auth:                harnessmodule.AuthManagedDevice,
		AuthInstructions:    "Starts `codex login --device-auth` on the host. Sign in with ChatGPT so Codex uses subscription limits instead of API-key billing.",
		SatisfiesAccessGate: true,
		LegacySkillRoots: []string{
			"/root/.codex/skills",
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
		auth, err := NewAuth(deps.Accounts)
		if err != nil {
			return harnessmodule.Components{}, err
		}
		binding := harnessauth.NewDeviceBinding(harness.ProviderCodex, auth.device).WithWarning(func() string {
			if auth.Status().UsesAPIKey {
				return "Codex is logged in with an API key. Sign in with ChatGPT to use subscription limits."
			}
			return ""
		})
		// Attach only a real service: a nil pointer inside the interface
		// would still report saved accounts as available.
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
		CLIErrorOperation:  "codex CLI unavailable in container",
		BeforeCredentials:  validateSubscriptionCredentials,
		SkillLinksRequired: true,
		BrowserAssets:      true,
		BrowserMCPRuntime:  true,
	}))
}

var (
	_ harness.Provider             = (*Provider)(nil)
	_ harnessmodule.FactoryBuilder = NewFactory
)
