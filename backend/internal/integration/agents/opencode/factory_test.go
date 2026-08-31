package opencode

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
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
	if descriptor.Features.Skills != agentmodule.SkillsInstructions {
		t.Fatalf("skills strategy = %q", descriptor.Features.Skills)
	}
	if !descriptor.Features.ScheduledTools {
		t.Fatal("opencode must declare scheduled tools support")
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

func TestBuildCmdPassesRuntimeEnvironmentOnHost(t *testing.T) {
	runtimeEnv := map[string]string{
		"REMOTE_SCHEDULE_API":   "https://remote.test/agent-api/schedules",
		"REMOTE_SCHEDULE_GRANT": "short-lived-grant",
	}
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	request := agent.RunRequest{
		Prompt:     "resume",
		Cwd:        t.TempDir(),
		RuntimeEnv: runtimeEnv,
	}
	cmd, containerName, err := provider.buildCmd(
		context.Background(),
		request,
		provider.args(request),
		func(agent.Event) {},
	)
	if err != nil {
		t.Fatal(err)
	}
	if containerName != "" {
		t.Fatalf("host command container = %q", containerName)
	}
	for key, value := range runtimeEnv {
		if !slices.Contains(cmd.Env, key+"="+value) {
			t.Fatalf("host command env missing %s: %#v", key, cmd.Env)
		}
	}
}

func newTestProvider(
	projects agent.ProjectResolver,
	dependencies provisioning.ContainerDependencies,
) *Provider {
	factory, err := NewFactory()
	if err != nil {
		panic(err)
	}
	catalog, err := agentmodule.NewCatalog(factory)
	if err != nil {
		panic(err)
	}
	runtime, err := catalog.Build(agentmodule.BuildDependencies{
		Projects:              projects,
		Containers:            dependencies,
		CredentialSyncTimeout: 30 * time.Second,
	})
	if err != nil {
		panic(err)
	}
	return runtime.Lookup(agent.ProviderOpenCode).(*Provider)
}
