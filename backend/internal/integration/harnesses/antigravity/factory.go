package antigravity

import (
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	harnessmodule "github.com/futrx-com/remote.futrx.com/internal/service/harness/module"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

// NewFactory returns Antigravity's complete module definition. Authentication is
// provider-owned and external, while the shared profile supplies deterministic
// host and project provisioning policy.
func NewFactory() (harnessmodule.Factory, error) {
	profile := Profile()
	return harnessmodule.NewFactory(harnessmodule.Descriptor{
		ID:               harness.ProviderAntigravity,
		Label:            "Antigravity",
		ExecutionScopes:  []harnessmodule.ExecutionScope{harnessmodule.ScopeHost, harnessmodule.ScopeProject},
		Auth:             harnessmodule.AuthExternal,
		AuthInstructions: "Open the project's Terminal application, run `agy`, and complete its sign-in flow.",
		Features: harnessmodule.Features{
			Sessions:              harnessmodule.SessionSupport{Resume: true},
			Skills:                harnessmodule.SkillsInstructions,
			ApplicationTools:      true,
			StreamingPresentation: harnessmodule.StreamingBlocks,
		},
	}, &profile, func(deps harnessmodule.Dependencies, validatedProfile *provisioning.Profile) (harnessmodule.Components, error) {
		binding := harnessauth.NewExternalBinding(harness.ProviderAntigravity)
		return harnessmodule.Components{
			Provider: newProvider(
				deps.ProjectPreparer,
				validatedProfile.CLI.Binary,
				deps.AccountAuthorizer,
			),
			Auth: &binding,
		}, nil
	})
}

var (
	_ harness.Provider             = (*Provider)(nil)
	_ harnessmodule.FactoryBuilder = NewFactory
)
