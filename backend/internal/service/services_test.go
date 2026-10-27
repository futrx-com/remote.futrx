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
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filesessions"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filetwofactor"
)

type stubCLIProvisioner struct{}

func (stubCLIProvisioner) Ensure(context.Context, string, provisioning.CLISpec) error { return nil }

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

func TestNewAuthAllowsLocalAdminWithoutGoogleOAuth(t *testing.T) {
	twoFactorStore, err := filetwofactor.New(t.TempDir())
	if err != nil {
		t.Fatalf("init two-factor store: %v", err)
	}
	sessionRegistryStore, err := filesessions.New(t.TempDir())
	if err != nil {
		t.Fatalf("init session registry store: %v", err)
	}
	auth, err := newAuth(
		context.Background(),
		fileauth.New(t.TempDir()),
		nil,
		"https://remote.example.com",
		twoFactorStore,
		sessionRegistryStore,
		AuthOptions{
			PendingLoginTTL:     5 * time.Minute,
			EnrollmentTTL:       10 * time.Minute,
			RecoveryCodeCount:   10,
			SessionHistoryLimit: 20,
			SetupTokenTTL:       30 * time.Minute,
		},
	)
	if err != nil {
		t.Fatalf("newAuth: %v", err)
	}
	if auth.GoogleOAuthEnabled() {
		t.Fatal("Google OAuth unexpectedly enabled")
	}
	if got := auth.SetupTokenTTL(); got != 30*time.Minute {
		t.Fatalf("setup token TTL = %s, want 30m", got)
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
