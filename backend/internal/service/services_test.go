package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/prompt"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	serviceschedule "github.com/futrx-com/remote.futrx.com/internal/service/schedule"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
)

type stubCLIProvisioner struct{}

func (stubCLIProvisioner) Ensure(context.Context, string, provisioning.CLISpec) error { return nil }

type contextAwareScheduleProvider struct {
	started chan struct{}
}

func (p *contextAwareScheduleProvider) ID() agent.ProviderID {
	return agent.ProviderCodex
}

func (p *contextAwareScheduleProvider) Parser(agent.RunRequest) agent.LineParser {
	return nil
}

func (p *contextAwareScheduleProvider) Run(
	ctx context.Context,
	_ agent.RunRequest,
	_ func(agent.Event),
) error {
	close(p.started)
	<-ctx.Done()
	return ctx.Err()
}

type staticScheduleToolIssuer struct{}

func (staticScheduleToolIssuer) IssueScheduleTool(
	context.Context,
	prompt.ScheduleToolRequest,
) (prompt.ScheduleToolAccess, error) {
	return prompt.ScheduleToolAccess{
		APIURL: "https://remote.example.com/agent-api/schedules",
		Token:  "test-token",
	}, nil
}

func TestNewAuthAllowsLocalAdminWithoutGoogleOAuth(t *testing.T) {
	auth, err := newAuth(
		context.Background(),
		fileauth.New(t.TempDir()),
		nil,
		"https://remote.example.com",
		nil,
	)
	if err != nil {
		t.Fatalf("newAuth: %v", err)
	}
	if auth.GoogleOAuthEnabled() {
		t.Fatal("Google OAuth unexpectedly enabled")
	}
}

func TestNewRejectsPartialAgentContainerDependencies(t *testing.T) {
	_, err := New(context.Background(), Dependencies{
		AgentContainers: provisioning.ContainerDependencies{CLI: stubCLIProvisioner{}},
	})
	if err == nil {
		t.Fatal("expected partial agent container dependencies to fail")
	}
	if !strings.Contains(err.Error(), "incomplete container dependencies") {
		t.Fatalf("New error = %q, want incomplete dependency error", err)
	}
}

func TestAgentProfilesComeFromRegistrationCatalog(t *testing.T) {
	// antigravity's CLI owns its auth (OS keyring / per-home token fallback
	// with no stable token path), so it is the one profile allowed to ship
	// without a credential sync policy — sign-in happens in the chat terminal.
	credentialExempt := map[string]bool{"antigravity": true}

	profiles := AgentProfiles()
	ids := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		ids = append(ids, profile.ID)
		if profile.CLI.Binary == "" {
			t.Fatalf("profile %q has incomplete CLI policy: %#v", profile.ID, profile.CLI)
		}
		if profile.CLI.InstallMode == provisioning.InstallWithScript {
			if profile.CLI.InstallScript == "" {
				t.Fatalf("profile %q uses script install without a script", profile.ID)
			}
		} else if profile.CLI.PackageName == "" {
			t.Fatalf("profile %q has incomplete CLI policy: %#v", profile.ID, profile.CLI)
		}
		if profile.Credentials.Empty() && !credentialExempt[profile.ID] {
			t.Fatalf("profile %q has no credential policy", profile.ID)
		}
	}
	if want := []string{"claude", "codex", "kimi", "antigravity"}; !slices.Equal(ids, want) {
		t.Fatalf("profile IDs = %v, want %v", ids, want)
	}
}

func TestAgentAuthBindingsComeFromRegistrationCatalog(t *testing.T) {
	definitions := agentDefinitions()
	registry := agentauth.NewRegistry()
	ids := make([]string, 0, len(definitions))
	// antigravity deliberately registers a service-less binding: agy's bare
	// TUI sign-in cannot run under the shared code/device auth services, so
	// in-app auth reports unavailable and sign-in happens in the terminal.
	availabilityExempt := map[agent.ProviderID]bool{agent.ProviderAntigravity: true}

	for _, definition := range definitions {
		binding := definition.authBinding()
		profile := definition.profile()
		if string(binding.ID()) != profile.ID {
			t.Fatalf("auth binding %q has profile %q", binding.ID(), profile.ID)
		}
		if !binding.Available() && !availabilityExempt[binding.ID()] {
			t.Fatalf("auth binding %q is unavailable", binding.ID())
		}
		if err := registry.Register(binding); err != nil {
			t.Fatalf("register auth binding %q: %v", binding.ID(), err)
		}
		ids = append(ids, string(binding.ID()))
	}
	if want := []string{"claude", "codex", "kimi", "antigravity"}; !slices.Equal(ids, want) {
		t.Fatalf("auth binding IDs = %v, want %v", ids, want)
	}
}

func TestAgentProfilesReturnsDefensiveCopies(t *testing.T) {
	first := AgentProfiles()
	first[0].Credentials.Files[0].HostPath = "/changed"
	first[0].BrowserMCPTemplates[0].Content[0] = 'x'

	second := AgentProfiles()
	if second[0].Credentials.Files[0].HostPath == "/changed" {
		t.Fatal("credential policy mutation escaped the catalog")
	}
	if second[0].BrowserMCPTemplates[0].Content[0] == 'x' {
		t.Fatal("template mutation escaped the catalog")
	}
}

func TestScheduledPromptExecutorPropagatesSchedulerCancellation(t *testing.T) {
	t.Parallel()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	chat, err := store.Create(context.Background(), servicechat.Meta{
		ID:        "aabbcc55",
		Provider:  servicechat.ProviderCodex,
		Cwd:       t.TempDir(),
		ProjectID: "project-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &contextAwareScheduleProvider{started: make(chan struct{})}
	agents := agent.NewRegistry()
	if err := agents.Register(provider); err != nil {
		t.Fatal(err)
	}
	prompts := prompt.New(
		store,
		nil,
		nil,
		runhub.New(store),
		agents,
		prompt.WithScheduleToolIssuer(staticScheduleToolIssuer{}),
	)
	executor := scheduledPromptExecutor{prompts: prompts}
	ctx, cancel := context.WithCancel(context.Background())
	handle, err := executor.StartScheduledPrompt(ctx, serviceschedule.Task{
		ID:          "0123456789abcdef01234567",
		OwnerEmail:  "owner@example.com",
		ChatID:      chat.ID,
		ActiveRunID: "schedule-run-1",
	}, "continue")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled provider did not start")
	}

	cancel()
	select {
	case result := <-handle.Done():
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("run error = %v, want context canceled", result.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled prompt ignored scheduler cancellation")
	}
}
