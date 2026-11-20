package minimax

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	harnessmodule "github.com/futrx-com/remote.futrx.com/internal/service/harness/module"
)

func TestFactoryDeclaresProjectCodexHarnessFeatures(t *testing.T) {
	factory, err := NewFactory()
	if err != nil {
		t.Fatal(err)
	}
	descriptor := factory.Descriptor()
	if descriptor.ID != harness.ProviderMiniMax || descriptor.Label != "MiniMax" || descriptor.Default {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	if len(descriptor.ExecutionScopes) != 1 || descriptor.ExecutionScopes[0] != harnessmodule.ScopeProject {
		t.Fatalf("execution scopes = %#v", descriptor.ExecutionScopes)
	}
	if descriptor.Auth != harnessmodule.AuthManagedAPIKey || descriptor.SatisfiesAccessGate {
		t.Fatalf("auth policy = %#v", descriptor)
	}
	if descriptor.APIKeyAuth == nil || descriptor.APIKeyAuth.CreateURL == "" ||
		descriptor.APIKeyAuth.CreateLabel == "" || descriptor.APIKeyAuth.CredentialLabel == "" {
		t.Fatalf("API key policy = %#v", descriptor.APIKeyAuth)
	}
	if !descriptor.Features.Sessions.Resume || !descriptor.Features.Sessions.Fork ||
		descriptor.Features.Skills != harnessmodule.SkillsDollarMention ||
		!descriptor.Features.BrowserTools || !descriptor.Features.ApplicationTools ||
		!descriptor.Features.ExecutionPolicies {
		t.Fatalf("features = %#v", descriptor.Features)
	}

	catalog, err := harnessmodule.NewCatalog(factory)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := catalog.Build(harnessmodule.BuildDependencies{})
	if err != nil {
		t.Fatal(err)
	}
	provider := runtime.Lookup(harness.ProviderMiniMax)
	if provider == nil || provider.ID() != harness.ProviderMiniMax {
		t.Fatalf("provider = %#v", provider)
	}
	binding, ok := runtime.AuthBinding(harness.ProviderMiniMax)
	if !ok || binding.Flow() != harnessauth.FlowAPIKey {
		t.Fatalf("binding = (%#v, %t)", binding, ok)
	}
}
