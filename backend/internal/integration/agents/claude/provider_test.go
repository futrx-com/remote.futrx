package claude

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
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
	return runtime.Lookup(agent.ProviderClaude).(*Provider)
}

func TestArgsUseDesktopLikeClaudeHeadlessMode(t *testing.T) {
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	args := provider.args(agent.RunRequest{Model: "sonnet[1m]", ResumeID: "session-123"})

	want := []string{
		"-p",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
		"--dangerously-skip-permissions",
		"--model", "sonnet[1m]",
		"--resume", "session-123",
	}
	if !slices.Equal(args, want) {
		t.Fatalf("args mismatch\n got: %#v\nwant: %#v", args, want)
	}
	if slices.Contains(args, "--bare") {
		t.Fatal("claude provider must not use --bare for desktop-like behavior")
	}
}

func TestArgsUseNativePlanModeWithoutPermissionBypass(t *testing.T) {
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	args := provider.args(agent.RunRequest{Mode: agent.RunModePlan})

	modeIndex := slices.Index(args, "--permission-mode")
	if modeIndex < 0 || modeIndex+1 >= len(args) || args[modeIndex+1] != string(agent.RunModePlan) {
		t.Fatalf("native Plan mode missing: %#v", args)
	}
	if slices.Contains(args, "--dangerously-skip-permissions") {
		t.Fatalf("Plan mode must not bypass Claude permissions: %#v", args)
	}
}

func TestArgsIncludeBrowserMCPConfigOnlyWhenEnabled(t *testing.T) {
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	withoutBrowser := provider.args(agent.RunRequest{})
	if slices.Contains(withoutBrowser, "--mcp-config") {
		t.Fatalf("unexpected browser MCP config: %#v", withoutBrowser)
	}

	withBrowser := provider.args(agent.RunRequest{EnableBrowser: true})
	configIndex := slices.Index(withBrowser, "--mcp-config")
	if configIndex < 0 || configIndex+1 >= len(withBrowser) {
		t.Fatalf("missing --mcp-config pair: %#v", withBrowser)
	}
	if withBrowser[configIndex+1] != browserMCPConfigPath {
		t.Fatalf("mcp config path = %q, want %q", withBrowser[configIndex+1], browserMCPConfigPath)
	}
}

func TestArgsIncludeReasoningEffort(t *testing.T) {
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	args := provider.args(agent.RunRequest{
		Model: "opus",
		Preferences: agent.RunPreferences{
			ReasoningEffort: "max",
		},
	})

	effortIndex := slices.Index(args, "--effort")
	if effortIndex < 0 || effortIndex+1 >= len(args) {
		t.Fatalf("missing --effort pair: %#v", args)
	}
	if args[effortIndex+1] != "max" {
		t.Fatalf("--effort = %q, want %q", args[effortIndex+1], "max")
	}
}

func TestArgsEnableUltracodeForRun(t *testing.T) {
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	args := provider.args(agent.RunRequest{
		Model: "opus",
		Preferences: agent.RunPreferences{
			ReasoningEffort: ultracodeEffort,
		},
	})

	effortIndex := slices.Index(args, "--effort")
	if effortIndex < 0 || effortIndex+1 >= len(args) {
		t.Fatalf("missing --effort pair: %#v", args)
	}
	if args[effortIndex+1] != ultracodeEffort {
		t.Fatalf("--effort = %q, want %q", args[effortIndex+1], ultracodeEffort)
	}
}

func TestArgsIgnoreInvalidReasoningEffort(t *testing.T) {
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	args := provider.args(agent.RunRequest{
		Model: "opus",
		Preferences: agent.RunPreferences{
			ReasoningEffort: "extreme;invalid",
		},
	})
	if slices.Contains(args, "--effort") {
		t.Fatalf("unexpected --effort for invalid effort: %#v", args)
	}
}

func TestArgsEnableFastModeForRun(t *testing.T) {
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	args := provider.args(agent.RunRequest{
		Model: "opus",
		Preferences: agent.RunPreferences{
			ServiceTier: agent.ServiceTier(fastServiceTier),
		},
	})

	settingsIndex := slices.Index(args, "--settings")
	if settingsIndex < 0 || settingsIndex+1 >= len(args) {
		t.Fatalf("missing --settings pair: %#v", args)
	}
	if args[settingsIndex+1] != `{"fastMode":true}` {
		t.Fatalf("--settings = %q, want fast mode", args[settingsIndex+1])
	}
}

func TestArgsIgnoreUnsupportedServiceTier(t *testing.T) {
	provider := newTestProvider(nil, provisioning.ContainerDependencies{})
	args := provider.args(agent.RunRequest{
		Preferences: agent.RunPreferences{ServiceTier: "priority"},
	})
	if slices.Contains(args, "--settings") {
		t.Fatalf("unexpected --settings for unsupported service tier: %#v", args)
	}
}

func TestBuildCmdProvisionsBrowserMCPOnlyWhenEnabled(t *testing.T) {
	project := agent.Project{
		ID:            agent.ProjectID("abcd"),
		ContainerName: "browser-project",
		Status:        agent.ProjectStatusRunning,
	}
	projects := fakeClaudeProjects{project: project}

	withoutBrowser := &fakeClaudeBrowser{}
	provider := newTestProvider(projects, claudeContainerDependencies(withoutBrowser))
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

	withBrowser := &fakeClaudeBrowser{}
	provider = newTestProvider(projects, claudeContainerDependencies(withBrowser))
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
		fakeClaudeProjects{
			project: project,
			secrets: []agent.ProjectSecret{{
				Key:   "REMOTE_SCHEDULE_API",
				Value: "https://attacker.invalid",
			}},
		},
		claudeContainerDependencies(&fakeClaudeBrowser{}),
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
		requireClaudeArgPair(t, containerCmd.Args, "--env", key+"="+value)
	}
	if slices.Contains(containerCmd.Args, "REMOTE_SCHEDULE_API=https://attacker.invalid") {
		t.Fatal("project secret overrode the backend-issued schedule API")
	}
}

func TestBuildCmdRejectsPartialContainerDependencies(t *testing.T) {
	project := agent.Project{
		ID:            agent.ProjectID("abcd"),
		ContainerName: "partial-dependencies",
		Status:        agent.ProjectStatusRunning,
	}
	provider := newTestProvider(
		fakeClaudeProjects{project: project},
		provisioning.ContainerDependencies{CLI: fakeClaudeCLI{}},
	)
	req := agent.RunRequest{ProjectID: string(project.ID)}

	_, _, err := provider.buildCmd(context.Background(), req, provider.args(req), func(agent.Event) {})
	if err == nil {
		t.Fatal("expected partial container dependencies to fail")
	}
	const want = "incomplete container dependencies: missing credentials, workspace, runtime assets, browser, schedule tools, lifecycle"
	if err.Error() != want {
		t.Fatalf("buildCmd error = %q, want %q", err, want)
	}
}

type fakeClaudeProjects struct {
	project agent.Project
	secrets []agent.ProjectSecret
}

func (f fakeClaudeProjects) Get(context.Context, agent.ProjectID) (agent.Project, error) {
	return f.project, nil
}

func (f fakeClaudeProjects) Start(context.Context, agent.ProjectID) (agent.Project, error) {
	return f.project, nil
}

func (f fakeClaudeProjects) ListSecrets(context.Context, agent.ProjectID) ([]agent.ProjectSecret, error) {
	return f.secrets, nil
}

type fakeClaudeCLI struct{}

func (fakeClaudeCLI) Ensure(context.Context, string, provisioning.CLISpec) error { return nil }

type fakeClaudeCredentials struct{}

func (fakeClaudeCredentials) Ensure(context.Context, string, provisioning.CredentialSpec) error {
	return nil
}

func (fakeClaudeCredentials) SyncFromContainer(context.Context, string, provisioning.CredentialSpec) error {
	return nil
}

type fakeClaudeWorkspace struct{}

func (fakeClaudeWorkspace) EnsureAgentInstructions(context.Context, string) error { return nil }

func (fakeClaudeWorkspace) EnsureSkillLinks(context.Context, string) error { return nil }

type fakeClaudeRuntimeAssets struct{}

func (fakeClaudeRuntimeAssets) Ensure(context.Context, string, []provisioning.RuntimeAsset) error {
	return nil
}

type fakeClaudeBrowser struct {
	agentBrowserMCPCalls  int
	agentBrowserCoreCalls int
}

func (f *fakeClaudeBrowser) EnsureSkill(context.Context, string) error { return nil }

func (f *fakeClaudeBrowser) EnsureScript(context.Context, string) error { return nil }

func (f *fakeClaudeBrowser) EnsureMCP(context.Context, string) error {
	f.agentBrowserMCPCalls++
	return nil
}

func (f *fakeClaudeBrowser) EnsureCore(context.Context, string) error {
	f.agentBrowserCoreCalls++
	return nil
}

type fakeClaudeLifecycle struct{}

func (fakeClaudeLifecycle) EnsureBootAutostart(context.Context, string) error { return nil }

type fakeClaudeScheduleTools struct{}

func (fakeClaudeScheduleTools) Ensure(context.Context, string) error { return nil }

func claudeContainerDependencies(browser provisioning.BrowserProvisioner) provisioning.ContainerDependencies {
	return provisioning.ContainerDependencies{
		CLI:           fakeClaudeCLI{},
		Credentials:   fakeClaudeCredentials{},
		Workspace:     fakeClaudeWorkspace{},
		RuntimeAssets: fakeClaudeRuntimeAssets{},
		Browser:       browser,
		ScheduleTools: fakeClaudeScheduleTools{},
		Lifecycle:     fakeClaudeLifecycle{},
	}
}

func requireClaudeArgPair(t *testing.T, args []string, first, second string) {
	t.Helper()
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return
		}
	}
	t.Fatalf("command args missing pair %q %q: %#v", first, second, args)
}
