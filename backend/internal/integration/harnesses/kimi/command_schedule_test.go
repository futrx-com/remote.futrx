package kimi

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessmodule "github.com/futrx-com/remote.futrx.com/internal/service/harness/module"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

func TestBuildCmdPassesRuntimeEnvironmentOnHostAndIntoContainer(t *testing.T) {
	runtimeEnv := map[string]string{
		"REMOTE_APPLICATION_API":   "https://remote.test/agent-api/applications",
		"REMOTE_APPLICATION_GRANT": "short-lived-grant",
	}

	hostProvider := newTestProvider(nil, provisioning.ContainerDependencies{})
	hostRequest := harness.RunRequest{
		Prompt:     "resume",
		Cwd:        t.TempDir(),
		RuntimeEnv: runtimeEnv,
	}
	hostCmd, containerName, err := hostProvider.buildCmd(
		context.Background(),
		hostRequest,
		hostProvider.args(hostRequest),
		func(harness.Event) {},
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

	project := harness.Project{
		ID:            harness.ProjectID("abcd"),
		ContainerName: "schedule-project",
		Status:        harness.ProjectStatusRunning,
	}
	containerProvider := newTestProvider(
		fakeKimiScheduleProjects{
			project: project,
			secrets: []harness.ProjectSecret{{
				Key:   "REMOTE_APPLICATION_API",
				Value: "https://attacker.invalid",
			}},
		},
		provisioning.ContainerDependencies{},
	)
	containerRequest := harness.RunRequest{
		Prompt:     "resume",
		ProjectID:  string(project.ID),
		RuntimeEnv: runtimeEnv,
	}
	containerCmd, containerName, err := containerProvider.buildCmd(
		context.Background(),
		containerRequest,
		containerProvider.args(containerRequest),
		func(harness.Event) {},
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
	if slices.Contains(containerCmd.Args, "REMOTE_APPLICATION_API=https://attacker.invalid") {
		t.Fatal("project secret overrode the backend-issued schedule API")
	}
}

func newTestProvider(
	projects harness.ProjectResolver,
	dependencies provisioning.ContainerDependencies,
) *Provider {
	factory, err := NewFactory()
	if err != nil {
		panic(err)
	}
	catalog, err := harnessmodule.NewCatalog(factory)
	if err != nil {
		panic(err)
	}
	runtime, err := catalog.Build(harnessmodule.BuildDependencies{
		Projects:              projects,
		Containers:            dependencies,
		CredentialSyncTimeout: 30 * time.Second,
	})
	if err != nil {
		panic(err)
	}
	return runtime.Lookup(harness.ProviderKimi).(*Provider)
}

type fakeKimiScheduleProjects struct {
	project harness.Project
	secrets []harness.ProjectSecret
}

func (f fakeKimiScheduleProjects) Get(
	context.Context,
	harness.ProjectID,
) (harness.Project, error) {
	return f.project, nil
}

func (f fakeKimiScheduleProjects) Start(
	context.Context,
	harness.ProjectID,
) (harness.Project, error) {
	return f.project, nil
}

func (f fakeKimiScheduleProjects) ListSecrets(
	context.Context,
	harness.ProjectID,
) ([]harness.ProjectSecret, error) {
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
