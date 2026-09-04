package antigravity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

func TestArgsComposition(t *testing.T) {
	p := &Provider{}

	base := p.args(agent.RunRequest{Prompt: "do the thing"})
	joined := strings.Join(base, " ")
	if !strings.Contains(joined, "--print do the thing") {
		t.Fatalf("args missing print prompt: %v", base)
	}
	if !strings.Contains(joined, "--dangerously-skip-permissions") {
		t.Fatalf("headless run must auto-approve tools: %v", base)
	}
	if !strings.Contains(joined, "--print-timeout") {
		t.Fatalf("args missing print timeout: %v", base)
	}
	if strings.Contains(joined, "--conversation") || strings.Contains(joined, "--model") {
		t.Fatalf("unexpected optional flags in %v", base)
	}

	full := p.args(agent.RunRequest{
		Prompt:      "next",
		Model:       "gemini-3-pro",
		Mode:        agent.RunModePlan,
		ResumeID:    "abc-123",
		Preferences: agent.RunPreferences{ReasoningEffort: "xhigh"},
	})
	joined = strings.Join(full, " ")
	for _, want := range []string{
		"--model gemini-3-pro",
		"--mode plan",
		"--conversation abc-123",
		"--effort high",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %q: %v", want, full)
		}
	}
	if strings.Contains(joined, "--dangerously-skip-permissions") {
		t.Fatalf("Plan mode must not bypass Antigravity permissions: %v", full)
	}
}

func TestEffortFlagClamping(t *testing.T) {
	tests := map[agent.ReasoningEffort]string{
		"":        "",
		"none":    "low",
		"minimal": "low",
		"low":     "low",
		"medium":  "medium",
		"high":    "high",
		"xhigh":   "high",
		"ultra":   "ultra",
		"bad;arg": "",
	}
	for effort, want := range tests {
		if got := effortFlag(effort); got != want {
			t.Fatalf("effortFlag(%q) = %q, want %q", effort, got, want)
		}
	}
}

func TestBuildCmdUsesAntigravityProjectPreparationPolicy(t *testing.T) {
	project := agent.Project{
		ID:            "project-id",
		ContainerName: "antigravity-project",
		Status:        "stopped",
	}
	calls := &antigravityPreparationCalls{}
	factory, err := NewFactory()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := agentmodule.NewCatalog(factory)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := catalog.Build(agentmodule.BuildDependencies{
		Projects: antigravityTestProjects{
			project: project,
			secrets: []agent.ProjectSecret{
				{Key: "REMOTE_SCHEDULE_API", Value: "https://attacker.invalid"},
				{Key: "SAFE_SECRET", Value: "safe"},
			},
			calls: calls,
		},
		Containers: antigravityContainerDependencies(calls),
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := runtime.Lookup(agent.ProviderAntigravity).(*Provider)
	request := agent.RunRequest{
		ProjectID:           string(project.ID),
		Prompt:              "test",
		EnableBrowser:       true,
		EnableScheduleTools: true,
		RuntimeEnv: map[string]string{
			"REMOTE_SCHEDULE_API": "https://remote.test/agent-api/schedules",
		},
	}
	var subtypes []string
	command, containerName, err := provider.buildCmd(
		context.Background(),
		request,
		provider.args(request),
		func(event agent.Event) { subtypes = append(subtypes, event.Subtype) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if containerName != project.ContainerName {
		t.Fatalf("container name = %q, want %q", containerName, project.ContainerName)
	}
	if !slices.Equal(subtypes, []string{"container_starting", "container_preparing"}) {
		t.Fatalf("preparation events = %v", subtypes)
	}
	if calls.start != 1 || calls.cli != 1 || calls.instructions != 1 ||
		calls.skillLinks != 1 || calls.schedule != 1 || calls.lifecycle != 1 {
		t.Fatalf("required preparation calls = %#v", calls)
	}
	if calls.credentials != 0 || calls.browserSkill != 0 || calls.browserScript != 0 ||
		calls.browserMCP != 0 || calls.browserCore != 0 {
		t.Fatalf("unsupported preparation calls = %#v", calls)
	}
	requireAntigravityArgPair(t, command.Args, "--env", "HOME=/root")
	requireAntigravityArgPair(t, command.Args, "--env", "SAFE_SECRET=safe")
	requireAntigravityArgPair(t, command.Args, "--env", "REMOTE_SCHEDULE_API=https://remote.test/agent-api/schedules")
	if slices.Contains(command.Args, "REMOTE_SCHEDULE_API=https://attacker.invalid") {
		t.Fatal("project secret overrode the backend-issued runtime environment")
	}
	if !slices.Contains(command.Args, project.ContainerName) || !slices.Contains(command.Args, "agy") {
		t.Fatalf("container command = %#v", command.Args)
	}
}

type antigravityPreparationCalls struct {
	start         int
	cli           int
	credentials   int
	instructions  int
	skillLinks    int
	browserSkill  int
	browserScript int
	browserMCP    int
	browserCore   int
	schedule      int
	lifecycle     int
}

type antigravityTestProjects struct {
	project agent.Project
	secrets []agent.ProjectSecret
	calls   *antigravityPreparationCalls
}

func (f antigravityTestProjects) Get(context.Context, agent.ProjectID) (agent.Project, error) {
	return f.project, nil
}

func (f antigravityTestProjects) Start(context.Context, agent.ProjectID) (agent.Project, error) {
	f.calls.start++
	return f.project, nil
}

func (f antigravityTestProjects) ListSecrets(context.Context, agent.ProjectID) ([]agent.ProjectSecret, error) {
	return f.secrets, nil
}

type antigravityTestCLI struct{ calls *antigravityPreparationCalls }

func (f antigravityTestCLI) Ensure(context.Context, string, provisioning.CLISpec) error {
	f.calls.cli++
	return nil
}

type antigravityTestCredentials struct{ calls *antigravityPreparationCalls }

func (f antigravityTestCredentials) Ensure(context.Context, string, provisioning.CredentialSpec) error {
	f.calls.credentials++
	return nil
}

func (f antigravityTestCredentials) SyncFromContainer(context.Context, string, provisioning.CredentialSpec) error {
	return nil
}

type antigravityTestWorkspace struct{ calls *antigravityPreparationCalls }

func (f antigravityTestWorkspace) EnsureAgentInstructions(context.Context, string) error {
	f.calls.instructions++
	return nil
}

func (f antigravityTestWorkspace) EnsureSkillLinks(context.Context, string) error {
	f.calls.skillLinks++
	return errors.New("stale skill link")
}

type antigravityTestRuntimeAssets struct{}

func (antigravityTestRuntimeAssets) Ensure(context.Context, string, []provisioning.RuntimeAsset) error {
	return nil
}

type antigravityTestBrowser struct{ calls *antigravityPreparationCalls }

func (f antigravityTestBrowser) EnsureSkill(context.Context, string) error {
	f.calls.browserSkill++
	return nil
}

func (f antigravityTestBrowser) EnsureScript(context.Context, string) error {
	f.calls.browserScript++
	return nil
}

func (f antigravityTestBrowser) EnsureMCP(context.Context, string) error {
	f.calls.browserMCP++
	return nil
}

func (f antigravityTestBrowser) EnsureCore(context.Context, string) error {
	f.calls.browserCore++
	return nil
}

type antigravityTestSchedule struct{ calls *antigravityPreparationCalls }

func (f antigravityTestSchedule) Ensure(context.Context, string) error {
	f.calls.schedule++
	return nil
}

type antigravityTestLifecycle struct{ calls *antigravityPreparationCalls }

func (f antigravityTestLifecycle) EnsureBootAutostart(context.Context, string) error {
	f.calls.lifecycle++
	return nil
}

func antigravityContainerDependencies(calls *antigravityPreparationCalls) provisioning.ContainerDependencies {
	return provisioning.ContainerDependencies{
		CLI:           antigravityTestCLI{calls},
		Credentials:   antigravityTestCredentials{calls},
		Workspace:     antigravityTestWorkspace{calls},
		RuntimeAssets: antigravityTestRuntimeAssets{},
		Browser:       antigravityTestBrowser{calls},
		ScheduleTools: antigravityTestSchedule{calls},
		Lifecycle:     antigravityTestLifecycle{calls},
	}
}

func requireAntigravityArgPair(t *testing.T, args []string, first, second string) {
	t.Helper()
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return
		}
	}
	t.Fatalf("command args missing pair %q %q: %#v", first, second, args)
}

func TestInstallScriptPinsVersionedRelease(t *testing.T) {
	script := Profile().CLI.InstallScript
	version := Profile().CLI.Version
	for _, want := range []string{
		releaseBaseURL + "/" + version + "/${asset}",
		`asset="agy_cli_linux_x64.tar.gz"`,
		`asset="agy_cli_linux_arm64.tar.gz"`,
		provisioning.MustPin("ANTIGRAVITY_LINUX_X64_SHA512"),
		provisioning.MustPin("ANTIGRAVITY_LINUX_ARM64_SHA512"),
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("install script does not contain pinned release value %q", want)
		}
	}
	if !strings.Contains(script, "sha512sum -c") {
		t.Fatal("install script must verify the pinned checksum")
	}
	if !strings.Contains(script, "/usr/local/bin/agy") {
		t.Fatal("container install script must retain the standard agy default")
	}
	if !strings.Contains(script, "FUTRX_HOST_CLI_INSTALL_PATH") ||
		!strings.Contains(script, `install -m 0755 "$tmp/antigravity" "$install_path"`) {
		t.Fatal("install script must honor the host-managed executable path")
	}
	if strings.Contains(script, "/manifests/") {
		t.Fatal("install script must not consult the moving latest manifest")
	}
}

func TestParserEmitsTextDeltas(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "c1"})
	events, err := parser.ParseLine([]byte("hello world"))
	if err != nil || len(events) != 1 {
		t.Fatalf("ParseLine = (%v, %v)", events, err)
	}
	if events[0].Type != agent.EventAssistantTextDelta || events[0].Text != "hello world\n" {
		t.Fatalf("unexpected event: %#v", events[0])
	}
}

func TestConversationDiscovery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	brain := filepath.Join(home, stateDirUnderHome, "brain")
	if err := os.MkdirAll(filepath.Join(brain, "0aa89f21-1111-4222-8333-abcdef012345"), 0o755); err != nil {
		t.Fatal(err)
	}

	store := conversationStore{}
	before := store.list(context.Background())
	if len(before) != 1 {
		t.Fatalf("expected 1 existing conversation, got %d", len(before))
	}

	// No new conversation yet -> ambiguous/none.
	if id := store.newConversation(context.Background(), before); id != "" {
		t.Fatalf("expected no new conversation, got %q", id)
	}

	fresh := "1bb99a32-2222-4333-9444-bcdef0123456"
	if err := os.MkdirAll(filepath.Join(brain, fresh), 0o755); err != nil {
		t.Fatal(err)
	}
	if id := store.newConversation(context.Background(), before); id != fresh {
		t.Fatalf("newConversation = %q, want %q", id, fresh)
	}

	// Junk entries are ignored.
	if err := os.MkdirAll(filepath.Join(brain, "not a conversation!"), 0o755); err != nil {
		t.Fatal(err)
	}
	if id := store.newConversation(context.Background(), before); id != fresh {
		t.Fatalf("junk entry changed discovery: %q", id)
	}
}
