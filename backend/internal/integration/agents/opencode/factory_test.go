package opencode

import (
	"slices"
	"testing"

	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

func TestFactoryDeclaresHostAndProjectScopes(t *testing.T) {
	factory, err := NewFactory()
	if err != nil {
		t.Fatal(err)
	}
	descriptor := factory.Descriptor()
	if !slices.Contains(descriptor.ExecutionScopes, agentmodule.ScopeHost) ||
		!slices.Contains(descriptor.ExecutionScopes, agentmodule.ScopeProject) {
		t.Fatalf("execution scopes = %#v", descriptor.ExecutionScopes)
	}
	if !descriptor.Features.Sessions.Resume {
		t.Fatal("opencode must declare session resume")
	}
}

func TestProfileCredentialsTargetContainerAuthJSON(t *testing.T) {
	profile := Profile()
	file := profile.Credentials.Files[0]
	if file.ContainerPath != containerOpenCodeAuth || !file.PushRequired || !file.PullRequired {
		t.Fatalf("container credential policy = %#v", file)
	}
	if !profile.Credentials.SeedOnLaunch {
		t.Fatal("project containers must be seeded with auth.json before a run")
	}
}
