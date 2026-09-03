package kimi

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

func TestBuildCmdPassesRuntimeEnvironmentOnHostAndIntoContainer(t *testing.T) {
	runtimeEnv := map[string]string{
		"REMOTE_SCHEDULE_API":   "https://remote.test/agent-api/schedules",
		"REMOTE_SCHEDULE_GRANT": "short-lived-grant",
	}

	hostProvider := newTestProvider(nil, provisioning.ContainerDependencies{})
	hostRequest := agent.RunRequest{
		Prompt:     "resume",
		Cwd:        t.TempDir(),
		RuntimeEnv: runtimeEnv,
	}
	hostCmd, containerName, err := hostProvider.buildCmd(
		context.Background(),
		hostRequest,
		hostProvider.args(hostRequest),
		func(agent.Event) {},
	)
	if err != nil {
		t.Fatal(err)
	}
	if containerName != "" {
		t.Fatalf("host command container = %q", containerName)
	}
	for key, value := range runtimeEnv {
		if !slices.Contains(hostCmd.Env, key+"="+value) {
			t.Fatalf("host command env missing %s: %#v", key, hostCmd.Env)
		}
	}

	project := agent.Project{
		ID:            agent.ProjectID("abcd"),
		ContainerName: "schedule-project",
		Status:        agent.ProjectStatusRunning,
	}
	containerProvider := newTestProvider(
		fakeKimiScheduleProjects{
			project: project,
			secrets: []agent.ProjectSecret{{
				Key:   "REMOTE_SCHEDULE_API",
				Value: "https://attacker.invalid",
			}},
		},
		provisioning.ContainerDependencies{},
	)
	containerRequest := agent.RunRequest{
		Prompt:     "resume",
		ProjectID:  string(project.ID),
		RuntimeEnv: runtimeEnv,
	}
	containerCmd, containerName, err := containerProvider.buildCmd(
		context.Background(),
		containerRequest,
		containerProvider.args(containerRequest),
		func(agent.Event) {},
	)
	if err != nil {
		t.Fatal(err)
	}
	if containerName != project.ContainerName {
		t.Fatalf("container name = %q, want %q", containerName, project.ContainerName)
	}
	for key, value := range runtimeEnv {
		requireKimiArgPair(t, containerCmd.Args, "--env", key+"="+value)
	}
	if slices.Contains(containerCmd.Args, "REMOTE_SCHEDULE_API=https://attacker.invalid") {
		t.Fatal("project secret overrode the backend-issued schedule API")
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
	return runtime.Lookup(agent.ProviderKimi).(*Provider)
}

type fakeKimiScheduleProjects struct {
	project agent.Project
	secrets []agent.ProjectSecret
}

func (f fakeKimiScheduleProjects) Get(
	context.Context,
	agent.ProjectID,
) (agent.Project, error) {
	return f.project, nil
}

func (f fakeKimiScheduleProjects) Start(
	context.Context,
	agent.ProjectID,
) (agent.Project, error) {
	return f.project, nil
}

func (f fakeKimiScheduleProjects) ListSecrets(
	context.Context,
	agent.ProjectID,
) ([]agent.ProjectSecret, error) {
	return f.secrets, nil
}

func requireKimiArgPair(t *testing.T, args []string, first, second string) {
	t.Helper()
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return
		}
	}
	t.Fatalf("command args missing pair %q %q: %#v", first, second, args)
}
