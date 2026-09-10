package devin

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

func TestFactoryDeclaresDevinFeatures(t *testing.T) {
	factory, err := NewFactory()
	if err != nil {
		t.Fatal(err)
	}
	descriptor := factory.Descriptor()
	if descriptor.ID != agent.ProviderDevin || descriptor.Label != "Devin" || descriptor.Default {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	if len(descriptor.ExecutionScopes) != 2 ||
		descriptor.ExecutionScopes[0] != agentmodule.ScopeHost ||
		descriptor.ExecutionScopes[1] != agentmodule.ScopeProject {
		t.Fatalf("execution scopes = %#v", descriptor.ExecutionScopes)
	}
	if descriptor.Auth != agentmodule.AuthManagedCode || !descriptor.SatisfiesAccessGate {
		t.Fatalf("auth policy = %#v", descriptor)
	}
	if !descriptor.Features.Sessions.Resume || descriptor.Features.Sessions.Fork ||
		descriptor.Features.Skills != agentmodule.SkillsInstructions ||
		descriptor.Features.BrowserTools || !descriptor.Features.ScheduledTools {
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
	provider := runtime.Lookup(agent.ProviderDevin)
	if provider == nil || provider.ID() != agent.ProviderDevin {
		t.Fatalf("provider = %#v", provider)
	}
	binding, ok := runtime.AuthBinding(agent.ProviderDevin)
	if !ok || binding.Flow() != agentauth.FlowCode {
		t.Fatalf("binding = (%#v, %t)", binding, ok)
	}
}
