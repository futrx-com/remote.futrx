package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
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

type serviceTestProvider struct {
	id agent.ProviderID
}

func (p serviceTestProvider) ID() agent.ProviderID { return p.id }

func (p serviceTestProvider) Capabilities(context.Context, agent.CapabilityRequest) (agent.Capabilities, error) {
	return agent.Capabilities{Provider: p.id}, nil
}

func (p serviceTestProvider) Run(context.Context, agent.RunRequest, func(agent.Event)) error {
	return nil
}

func (p *contextAwareScheduleProvider) ID() agent.ProviderID {
	return agent.ProviderCodex
}

func (p *contextAwareScheduleProvider) Parser(agent.RunRequest) agent.LineParser {
	return nil
}

func (p *contextAwareScheduleProvider) Capabilities(context.Context, agent.CapabilityRequest) (agent.Capabilities, error) {
	return agent.Capabilities{Provider: agent.ProviderCodex}, nil
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

func TestNewRejectsAuthenticatedDeploymentWithoutAgentAccessGate(t *testing.T) {
	descriptor := agentmodule.Descriptor{
		ID:               "external-agent",
		Label:            "External Agent",
		ExecutionScopes:  []agentmodule.ExecutionScope{agentmodule.ScopeHost},
		Auth:             agentmodule.AuthExternal,
		AuthInstructions: "Authenticate outside Remote.",
		Features:         agentmodule.Features{Skills: agentmodule.SkillsNone},
	}
	factory, err := agentmodule.NewFactory(descriptor, nil, func(agentmodule.Dependencies, *provisioning.Profile) (agentmodule.Components, error) {
		binding := agentauth.NewExternalBinding(descriptor.ID)
		return agentmodule.Components{
			Provider: serviceTestProvider{id: descriptor.ID},
			Auth:     &binding,
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := agentmodule.NewCatalog(factory)
	if err != nil {
		t.Fatal(err)
	}

	_, err = New(context.Background(), Dependencies{
		Auth:         fileauth.New(t.TempDir()),
		AgentModules: catalog,
	})
	if !errors.Is(err, agentmodule.ErrNoAccessGate) {
		t.Fatalf("New error = %v, want ErrNoAccessGate", err)
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
