package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentexecution "github.com/futrx-com/remote.futrx.com/internal/service/agent/execution"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

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
	return runtime.Lookup(agent.ProviderCodex).(*Provider)
}

func TestArgsUseCodexAppServer(t *testing.T) {
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	args := provider.args(agent.RunRequest{Model: "gpt-5.5 [fast]"})

	want := []string{"app-server"}
	if !slices.Equal(args, want) {
		t.Fatalf("args mismatch\n got: %#v\nwant: %#v", args, want)
	}
}

func TestCodexEnvStripsOpenAIAPIKey(t *testing.T) {
	env := codexEnv([]string{
		"HOME=/root",
		"OPENAI_API_KEY=sk-test",
	})

	for _, item := range env {
		if strings.HasPrefix(item, "OPENAI_API_KEY=") {
			t.Fatalf("OPENAI_API_KEY leaked into codex env: %#v", env)
		}
	}
	if !slices.Contains(env, "CODEX_HOME=/root/.codex") {
		t.Fatalf("CODEX_HOME missing from env: %#v", env)
	}
}

func TestArgsIncludeBrowserMCPConfig(t *testing.T) {
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	args := provider.args(agent.RunRequest{EnableBrowser: true})

	want := []string{
		"app-server",
		"-c", `mcp_servers.browser.command="npx"`,
		"-c", `mcp_servers.browser.args=["@playwright/mcp","--cdp-endpoint","http://127.0.0.1:9222","--caps=vision"]`,
	}
	if !slices.Equal(args, want) {
		t.Fatalf("args mismatch\n got: %#v\nwant: %#v", args, want)
	}
}

func TestEnsureHostSubscriptionAuthRejectsAPIKeyAuth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"),
		[]byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"sk-test"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ensureHostSubscriptionAuth(); err == nil {
		t.Fatal("expected API key auth to be rejected")
	}
}

func TestContainerCredentialsRejectAPIKeyAuthBeforeProvisioning(t *testing.T) {
	authPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"sk-test"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	credentials := &fakeCodexCredentials{}
	profile := Profile()
	profile.Credentials.Files[0].HostPath = authPath
	project := agent.Project{
		ID:            agent.ProjectID("project-id"),
		ContainerName: "project",
		Status:        agent.ProjectStatusRunning,
	}
	preparer := agentexecution.New(
		fakeCodexProjects{project: project},
		codexContainerDependencies(credentials, &fakeCodexBrowser{}),
		agentexecution.Options{
			Provider:          agent.ProviderCodex,
			Profile:           profile,
			BeforeCredentials: validateSubscriptionCredentials,
		},
	)

	_, err := preparer.Prepare(context.Background(), agent.ProjectPreparationRequest{
		ProjectID: project.ID,
	}, func(agent.Event) {})
	if !errors.Is(err, ErrCodexAPIKeyAuth) {
		t.Fatalf("prepare project error = %v, want %v", err, ErrCodexAPIKeyAuth)
	}
	if credentials.ensureCalls != 0 {
		t.Fatalf("credentials provisioned despite API-key auth: %d", credentials.ensureCalls)
	}
}

func TestProfileReturnsIndependentProvisioningPolicy(t *testing.T) {
	first := Profile()
	first.Credentials.Files[0].HostPath = "/changed"

	second := Profile()
	if second.ID != "codex" {
		t.Fatalf("profile ID = %q, want codex", second.ID)
	}
	if second.CLI.NPMPackage() != "@openai/codex@"+provisioning.MustCLIVersion("CODEX_CLI_VERSION") {
		t.Fatalf("CLI package = %q", second.CLI.NPMPackage())
	}
	if second.Credentials.Files[0].HostPath != hostCodexAuth {
		t.Fatalf("profile mutation escaped clone: %q", second.Credentials.Files[0].HostPath)
	}
}

func TestBuildCmdProvisionsBrowserMCPOnlyWhenEnabled(t *testing.T) {
	project := agent.Project{
		ID:            agent.ProjectID("abcd"),
		ContainerName: "browser-project",
		Status:        agent.ProjectStatusRunning,
	}
	projects := fakeCodexProjects{project: project}

	withoutBrowser := &fakeCodexBrowser{}
	provider := newTestProvider(projects, codexContainerDependencies(nil, withoutBrowser))
	req := agent.RunRequest{ProjectID: string(project.ID)}
	if _, _, err := provider.buildCmd(context.Background(), req, provider.args(req), func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}
	if withoutBrowser.agentBrowserMCPCalls != 0 {
		t.Fatalf("browser MCP provisioned without browser skill: %d", withoutBrowser.agentBrowserMCPCalls)
	}
	if withoutBrowser.agentBrowserCoreCalls != 0 {
		t.Fatalf("browser core started without browser skill: %d", withoutBrowser.agentBrowserCoreCalls)
	}

	withBrowser := &fakeCodexBrowser{}
	provider = newTestProvider(projects, codexContainerDependencies(nil, withBrowser))
	req.EnableBrowser = true
	if _, _, err := provider.buildCmd(context.Background(), req, provider.args(req), func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}
	if withBrowser.agentBrowserMCPCalls != 1 {
		t.Fatalf("browser MCP calls = %d, want 1", withBrowser.agentBrowserMCPCalls)
	}
	if withBrowser.agentBrowserCoreCalls != 1 {
		t.Fatalf("browser core calls = %d, want 1", withBrowser.agentBrowserCoreCalls)
	}
}

func TestBuildCmdPassesRuntimeEnvironmentOnHostAndIntoContainer(t *testing.T) {
	runtimeEnv := map[string]string{
		"REMOTE_SCHEDULE_API":   "https://remote.test/agent-api/schedules",
		"REMOTE_SCHEDULE_GRANT": "short-lived-grant",
	}

	t.Setenv("CODEX_HOME", t.TempDir())
	hostProvider := newTestProvider(nil, provisioning.ContainerDependencies{})
	hostRequest := agent.RunRequest{Cwd: t.TempDir(), RuntimeEnv: runtimeEnv}
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
		fakeCodexProjects{
			project: project,
			secrets: []agent.ProjectSecret{{
				Key:   "REMOTE_SCHEDULE_API",
				Value: "https://attacker.invalid",
			}},
		},
		codexContainerDependencies(nil, &fakeCodexBrowser{}),
	)
	containerRequest := agent.RunRequest{
		ProjectID:           string(project.ID),
		RuntimeEnv:          runtimeEnv,
		EnableScheduleTools: true,
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
		requireCodexArgPair(t, containerCmd.Args, "--env", key+"="+value)
	}
	if slices.Contains(containerCmd.Args, "REMOTE_SCHEDULE_API=https://attacker.invalid") {
		t.Fatal("project secret overrode the backend-issued schedule API")
	}
}

func TestBuildCmdReconcilesContainerEvenWhenCachedStatusIsRunning(t *testing.T) {
	project := agent.Project{
		ID:            agent.ProjectID("abcd"),
		ContainerName: "recycled-project",
		Status:        agent.ProjectStatusRunning,
	}
	startCalls := 0
	provider := newTestProvider(fakeCodexProjects{project: project, startCalls: &startCalls}, provisioning.ContainerDependencies{})
	req := agent.RunRequest{ProjectID: string(project.ID)}

	if _, _, err := provider.buildCmd(context.Background(), req, provider.args(req), func(agent.Event) {}); err != nil {
		t.Fatal(err)
	}
	if startCalls != 1 {
		t.Fatalf("Start() calls = %d, want 1", startCalls)
	}
}

type fakeCodexProjects struct {
	project    agent.Project
	startCalls *int
	secrets    []agent.ProjectSecret
}

func (f fakeCodexProjects) Get(context.Context, agent.ProjectID) (agent.Project, error) {
	return f.project, nil
}

func (f fakeCodexProjects) Start(context.Context, agent.ProjectID) (agent.Project, error) {
	if f.startCalls != nil {
		(*f.startCalls)++
	}
	return f.project, nil
}

func (f fakeCodexProjects) ListSecrets(context.Context, agent.ProjectID) ([]agent.ProjectSecret, error) {
	return f.secrets, nil
}

type fakeCodexCLI struct{}

func (fakeCodexCLI) Ensure(context.Context, string, provisioning.CLISpec) error { return nil }

type fakeCodexCredentials struct {
	ensureCalls int
}

func (f *fakeCodexCredentials) Ensure(context.Context, string, provisioning.CredentialSpec) error {
	f.ensureCalls++
	return nil
}

func (f *fakeCodexCredentials) SyncFromContainer(context.Context, string, provisioning.CredentialSpec) error {
	return nil
}

type fakeCodexWorkspace struct{}

func (fakeCodexWorkspace) EnsureAgentInstructions(context.Context, string) error { return nil }

func (fakeCodexWorkspace) EnsureSkillLinks(context.Context, string) error { return nil }

type fakeCodexRuntimeAssets struct{}

func (fakeCodexRuntimeAssets) Ensure(context.Context, string, []provisioning.RuntimeAsset) error {
	return nil
}

type fakeCodexBrowser struct {
	agentBrowserMCPCalls  int
	agentBrowserCoreCalls int
}

func (f *fakeCodexBrowser) EnsureSkill(context.Context, string) error { return nil }

func (f *fakeCodexBrowser) EnsureScript(context.Context, string) error { return nil }

func (f *fakeCodexBrowser) EnsureMCP(context.Context, string) error {
	f.agentBrowserMCPCalls++
	return nil
}

func (f *fakeCodexBrowser) EnsureCore(context.Context, string) error {
	f.agentBrowserCoreCalls++
	return nil
}

type fakeCodexLifecycle struct{}

func (fakeCodexLifecycle) EnsureBootAutostart(context.Context, string) error { return nil }

type fakeCodexScheduleTools struct{}

func (fakeCodexScheduleTools) Ensure(context.Context, string) error { return nil }

func codexContainerDependencies(
	credentials *fakeCodexCredentials,
	browser provisioning.BrowserProvisioner,
) provisioning.ContainerDependencies {
	if credentials == nil {
		credentials = &fakeCodexCredentials{}
	}
	return provisioning.ContainerDependencies{
		CLI:           fakeCodexCLI{},
		Credentials:   credentials,
		Workspace:     fakeCodexWorkspace{},
		RuntimeAssets: fakeCodexRuntimeAssets{},
		Browser:       browser,
		ScheduleTools: fakeCodexScheduleTools{},
		Lifecycle:     fakeCodexLifecycle{},
	}
}

func requireCodexArgPair(t *testing.T, args []string, first, second string) {
	t.Helper()
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return
		}
	}
	t.Fatalf("command args missing pair %q %q: %#v", first, second, args)
}
