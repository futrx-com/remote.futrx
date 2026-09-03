package minimax

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

func TestFactoryDeclaresProjectCodexHarnessFeatures(t *testing.T) {
	factory, err := NewFactory()
	if err != nil {
		t.Fatal(err)
	}
	descriptor := factory.Descriptor()
	if descriptor.ID != agent.ProviderMiniMax || descriptor.Label != "MiniMax" || descriptor.Default {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	if len(descriptor.ExecutionScopes) != 1 || descriptor.ExecutionScopes[0] != agentmodule.ScopeProject {
		t.Fatalf("execution scopes = %#v", descriptor.ExecutionScopes)
	}
	if descriptor.Auth != agentmodule.AuthManagedAPIKey || descriptor.SatisfiesAccessGate {
		t.Fatalf("auth policy = %#v", descriptor)
	}
	if descriptor.APIKeyAuth == nil || descriptor.APIKeyAuth.CreateURL == "" ||
		descriptor.APIKeyAuth.CreateLabel == "" || descriptor.APIKeyAuth.CredentialLabel == "" {
		t.Fatalf("API key policy = %#v", descriptor.APIKeyAuth)
	}
	if !descriptor.Features.Sessions.Resume || !descriptor.Features.Sessions.Fork ||
		descriptor.Features.Skills != agentmodule.SkillsDollarMention ||
		!descriptor.Features.BrowserTools || !descriptor.Features.ScheduledTools {
		t.Fatalf("features = %#v", descriptor.Features)
	}

	catalog, err := agentmodule.NewCatalog(factory)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := catalog.Build(agentmodule.BuildDependencies{})
	if err != nil {
		t.Fatal(err)
	}
	provider := runtime.Lookup(agent.ProviderMiniMax)
	if provider == nil || provider.ID() != agent.ProviderMiniMax {
		t.Fatalf("provider = %#v", provider)
	}
	binding, ok := runtime.AuthBinding(agent.ProviderMiniMax)
	if !ok || binding.Flow() != agentauth.FlowAPIKey {
		t.Fatalf("binding = (%#v, %t)", binding, ok)
	}
}
