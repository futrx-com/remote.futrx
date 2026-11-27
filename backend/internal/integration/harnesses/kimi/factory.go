package kimi

import (
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	harnessmodule "github.com/futrx-com/remote.futrx.com/internal/service/harness/module"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

// NewFactory returns Kimi's complete module definition, including its runtime,
// authentication flow, feature declarations, and provisioning profile.
func NewFactory() (harnessmodule.Factory, error) {
	profile := Profile()
	return harnessmodule.NewFactory(harnessmodule.Descriptor{
		ID:                  harness.ProviderKimi,
		Label:               "Kimi",
		ExecutionScopes:     []harnessmodule.ExecutionScope{harnessmodule.ScopeHost, harnessmodule.ScopeProject},
		Auth:                harnessmodule.AuthManagedDevice,
		AuthInstructions:    "Starts `kimi login` on the host. Sign in with your Kimi Code subscription using the displayed device code.",
		SatisfiesAccessGate: true,
		Features: harnessmodule.Features{
			Sessions:              harnessmodule.SessionSupport{Resume: true},
			Skills:                harnessmodule.SkillsInstructions,
			ApplicationTools:      true,
			StreamingPresentation: harnessmodule.StreamingBlocks,
		},
	}, &profile, func(deps harnessmodule.Dependencies, validatedProfile *provisioning.Profile) (harnessmodule.Components, error) {
		binding := harnessauth.NewDeviceBinding(harness.ProviderKimi, NewAuth())
		return harnessmodule.Components{
			Provider: newProvider(
				deps.ProjectPreparer,
				deps.CredentialCollector,
				*validatedProfile,
				deps.CredentialSyncTimeout,
				deps.AccountAuthorizer,
			),
			Auth: &binding,
		}, nil
	}, harnessmodule.WithProjectPreparation(harnessmodule.ProjectPreparationPolicy{
		BrowserAssets: true,
	}))
}

var (
	_ harness.Provider             = (*Provider)(nil)
	_ harnessmodule.FactoryBuilder = NewFactory
)
