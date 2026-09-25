package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
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

func TestFactoryDeclaresCodexHarnessExecutionPolicies(t *testing.T) {
	factory, err := NewFactory()
	if err != nil {
		t.Fatal(err)
	}
	if !factory.Descriptor().Features.ExecutionPolicies {
		t.Fatal("Codex must expose the approval and sandbox policies supported by its harness")
	}
}

func TestFactoryAttachesSavedAccountsOnlyWithAVault(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	for _, tc := range []struct {
		name  string
		vault *agentauth.AccountVault
	}{
		{"without a vault", nil},
		{"with a vault", agentauth.NewAccountVault(&memoryAccountStore{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, err := NewFactory()
			if err != nil {
				t.Fatal(err)
			}
			catalog, err := agentmodule.NewCatalog(factory)
			if err != nil {
				t.Fatal(err)
			}
			runtime, err := catalog.Build(agentmodule.BuildDependencies{Accounts: tc.vault, CredentialSyncTimeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			binding, ok := runtime.AuthBinding(agent.ProviderCodex)
			if !ok {
				t.Fatal("Codex has no auth binding")
			}
			want := tc.vault != nil
			if binding.AccountsAvailable() != want || (binding.Snapshot().Accounts != nil) != want {
				t.Fatalf("accounts available = %v, want %v", binding.AccountsAvailable(), want)
			}
			if provider := runtime.Lookup(agent.ProviderCodex).(*Provider); (provider.accounts != nil) != want {
				t.Fatalf("provider accounts = %v, want attached %v", provider.accounts, want)
			}
		})
	}
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

func TestCodexAuthEnvForOverridesInheritedCredentialHome(t *testing.T) {
	env := codexAuthEnvFor([]string{
		"HOME=/root",
		"CODEX_HOME=/root/.codex",
		"OPENAI_API_KEY=sk-test",
	}, "/tmp/isolated-codex")
	if slices.Contains(env, "CODEX_HOME=/root/.codex") || !slices.Contains(env, "CODEX_HOME=/tmp/isolated-codex") {
		t.Fatalf("isolated auth env = %#v", env)
	}
	for _, item := range env {
		if strings.HasPrefix(item, "OPENAI_API_KEY=") {
			t.Fatalf("OPENAI_API_KEY leaked into isolated auth env: %#v", env)
		}
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

// A project container can write any login into its isolated auth.json. Only a
// refreshed login for the selected account may reach the vault, and the
// canonical host login is never changed by the run.
func TestContainerRunKeepsOnlyTheActiveAccountsLogin(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	hostPath := filepath.Join(codexHome, "auth.json")
	installFakeContainerAppServer(t)
	saved := codexTestCredential("personal", "saved")
	writeCodexHostCredential(t, hostPath, saved)
	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		ActiveAccountID: "personal",
		Accounts:        []agentauth.AccountRecord{{ID: "personal", Label: "Personal", Credential: saved}},
	}}
	auth := newTestAuth(t, store)
	var validated []string
	auth.credentials.validate = func(_ context.Context, credential json.RawMessage) (agentauth.ValidatedAccount, error) {
		validated = append(validated, string(credential))
		// Validation refreshes the login through the Codex app server.
		refreshed := bytes.Replace(credential, []byte(`"access_token":"`), []byte(`"access_token":"validated-`), 1)
		return agentauth.ValidatedAccount{Email: "personal@example.test", PlanType: "plus", Credential: refreshed}, nil
	}

	project := agent.Project{ID: agent.ProjectID("abcd"), ContainerName: "account-project", Status: agent.ProjectStatusRunning}
	credentials := &fakeCodexCredentials{}
	provider := newTestProvider(fakeCodexProjects{project: project}, codexContainerDependencies(credentials, &fakeCodexBrowser{}))
	provider.accounts = auth.accounts
	run := func(synced json.RawMessage) {
		t.Helper()
		credentials.synced = synced
		err := provider.Run(context.Background(), agent.RunRequest{
			ProjectID: string(project.ID), ConversationID: "chat-1", Prompt: "hello",
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	requireSaved := func(want json.RawMessage) {
		t.Helper()
		accounts := store.saved()
		if len(accounts.Accounts) != 1 || accounts.ActiveAccountID != "personal" ||
			string(accounts.Accounts[0].Credential) != string(want) {
			t.Fatalf("saved accounts = %#v, want credential %s", accounts, want)
		}
	}

	run(codexTestCredential("intruder", "intruder"))
	requireSaved(saved)
	requireCodexHostCredential(t, hostPath, saved)

	// Copying back an API-key login fails the sync, but the login it left
	// on the host is still replaced.
	run(json.RawMessage(`{"auth_mode":"apikey","OPENAI_API_KEY":"sk-test"}`))
	requireSaved(saved)
	requireCodexHostCredential(t, hostPath, saved)
	if len(validated) != 0 {
		t.Fatalf("logins for other accounts were validated: %q", validated)
	}

	rotated := codexTestCredential("personal", "rotated")
	run(rotated)
	want := codexTestCredential("personal", "validated-rotated")
	if len(validated) != 1 || validated[0] != string(rotated) {
		t.Fatalf("validated logins = %q, want the rotated login", validated)
	}
	requireSaved(want)
	requireCodexHostCredential(t, hostPath, saved)
}

// A plan belongs to the account that ran, so the windows a saved-account run
// reports carry that account rather than only the provider.
func TestRunAttributesPlanLimitsToTheAccountThatRan(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	installFakeContainerAppServerWith(t,
		`{"method":"account/rateLimits/updated","params":{"rateLimits":{"limitId":"codex","primary":{"usedPercent":25,"windowDurationMins":300}}}}`,
	)
	saved := codexTestCredential("personal", "saved")
	writeCodexHostCredential(t, filepath.Join(codexHome, "auth.json"), saved)
	store := &memoryAccountStore{accounts: agentauth.AccountSet{
		ActiveAccountID: "personal",
		Accounts:        []agentauth.AccountRecord{{ID: "personal", Label: "Personal", Credential: saved}},
	}}
	project := agent.Project{ID: agent.ProjectID("abcd"), ContainerName: "account-project", Status: agent.ProjectStatusRunning}
	provider := newTestProvider(fakeCodexProjects{project: project}, codexContainerDependencies(&fakeCodexCredentials{}, &fakeCodexBrowser{}))
	provider.accounts = newTestAuth(t, store).accounts

	var quotas []agent.Event
	err := provider.Run(context.Background(), agent.RunRequest{
		ProjectID: string(project.ID), ConversationID: "chat-1", Prompt: "hello",
	}, func(event agent.Event) {
		if event.Type == agent.EventQuotaUpdated {
			quotas = append(quotas, event)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(quotas) != 1 || quotas[0].Provider != agent.ProviderCodex || quotas[0].AccountID != "personal" ||
		quotas[0].Quota == nil || *quotas[0].Quota.UsedPercent != 25 {
		t.Fatalf("quota events = %#v; want one reading for the personal account", quotas)
	}
}

func TestAccountRunHomesSeparateChats(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	saved := agentauth.RunCredential{AccountID: "personal", Credential: codexTestCredential("personal", "token")}
	first, err := newAccountRun(agent.RunRequest{ConversationID: "chat-one", ProjectID: "project"}, saved)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newAccountRun(agent.RunRequest{ConversationID: "chat-two", ProjectID: "project"}, saved)
	if err != nil {
		t.Fatal(err)
	}
	if first.hostHome == second.hostHome || first.containerHome == second.containerHome {
		t.Fatalf("chat account homes overlap: first %#v, second %#v", first, second)
	}
	requireCodexHostCredential(t, filepath.Join(first.hostHome, "auth.json"), saved.Credential)
	requireCodexHostCredential(t, filepath.Join(second.hostHome, "auth.json"), saved.Credential)
}

func TestBuildCmdUsesTheSelectedChatsPrivateAccountHome(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	project := agent.Project{ID: "project", ContainerName: "account-project", Status: agent.ProjectStatusRunning}
	credentials := &fakeCodexCredentials{}
	provider := newTestProvider(fakeCodexProjects{project: project}, codexContainerDependencies(credentials, &fakeCodexBrowser{}))
	req := agent.RunRequest{ProjectID: string(project.ID), ConversationID: "chat-one"}
	run, err := newAccountRun(req, agentauth.RunCredential{
		AccountID: "personal", Credential: codexTestCredential("personal", "token"),
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd, _, err := provider.buildCmdForAccount(context.Background(), req, provider.args(req), nil, run)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.ensured.ContainerDir != run.containerHome ||
		credentials.ensured.Files[0].ContainerPath != filepath.Join(run.containerHome, "auth.json") {
		t.Fatalf("prepared credential spec = %#v, run = %#v", credentials.ensured, run)
	}
	requireCodexArgPair(t, cmd.Args, "--env", "CODEX_HOME="+run.containerHome)
	if !slices.Contains(cmd.Args, codexAccountCommand) {
		t.Fatalf("container command does not initialize the private account home: %#v", cmd.Args)
	}
}

// installFakeContainerAppServer puts an lxc command first on PATH that ignores
// its arguments and completes one Codex app-server turn.
func installFakeContainerAppServer(t *testing.T) {
	t.Helper()
	installFakeContainerAppServerWith(t)
}

// installFakeContainerAppServerWith is installFakeContainerAppServer whose turn
// also sends each notification line before it completes.
func installFakeContainerAppServerWith(t *testing.T, notifications ...string) {
	t.Helper()
	binDir := t.TempDir()
	var sent strings.Builder
	for _, notification := range notifications {
		sent.WriteString("      printf '%s\\n' '" + notification + "'\n")
	}
	script := `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    *'"id":2'*) printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-1"},"model":"gpt-test"}}' ;;
    *'"id":3'*)
      printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-1","status":"inProgress","items":[]}}}'
` + sent.String() + `      printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}'
      exit 0
      ;;
  esac
done
`
	if err := os.WriteFile(filepath.Join(binDir, "lxc"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
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
	ensured     provisioning.CredentialSpec
	// synced, when set, is the login a run copies back from the container
	// over the host credential.
	synced json.RawMessage
}

func (f *fakeCodexCredentials) Ensure(_ context.Context, _ string, spec provisioning.CredentialSpec) error {
	f.ensureCalls++
	f.ensured = spec.Clone()
	return nil
}

func (f *fakeCodexCredentials) SyncFromContainer(_ context.Context, _ string, spec provisioning.CredentialSpec) error {
	if f.synced == nil {
		return nil
	}
	return agentauth.WriteCredentialFile(spec.Files[0].HostPath, f.synced)
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
