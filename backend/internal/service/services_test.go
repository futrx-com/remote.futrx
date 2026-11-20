package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	harnessmodule "github.com/futrx-com/remote.futrx.com/internal/service/harness/module"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filesessions"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filetwofactor"
)

type stubCLIProvisioner struct{}

func (stubCLIProvisioner) Ensure(context.Context, string, provisioning.CLISpec) error { return nil }

type serviceTestProvider struct {
	id harness.ProviderID
}

func (p serviceTestProvider) ID() harness.ProviderID { return p.id }

func (p serviceTestProvider) Capabilities(context.Context, harness.CapabilityRequest) (harness.Capabilities, error) {
	return harness.Capabilities{Provider: p.id}, nil
}

func (p serviceTestProvider) Run(context.Context, harness.RunRequest, func(harness.Event)) error {
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
	auth, err := NewAuth(
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
		t.Fatalf("NewAuth: %v", err)
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
	descriptor := harnessmodule.Descriptor{
		ID:               "external-agent",
		Label:            "External Agent",
		ExecutionScopes:  []harnessmodule.ExecutionScope{harnessmodule.ScopeHost},
		Auth:             harnessmodule.AuthExternal,
		AuthInstructions: "Authenticate outside Remote.",
		Features:         harnessmodule.Features{Skills: harnessmodule.SkillsNone},
	}
	factory, err := harnessmodule.NewFactory(descriptor, nil, func(harnessmodule.Dependencies, *provisioning.Profile) (harnessmodule.Components, error) {
		binding := harnessauth.NewExternalBinding(descriptor.ID)
		return harnessmodule.Components{
			Provider: serviceTestProvider{id: descriptor.ID},
			Auth:     &binding,
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := harnessmodule.NewCatalog(factory)
	if err != nil {
		t.Fatal(err)
	}

	twoFactorStore, err := filetwofactor.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sessionRegistryStore, err := filesessions.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuth(
		context.Background(), fileauth.New(t.TempDir()), nil, "https://remote.example.com",
		twoFactorStore, sessionRegistryStore, AuthOptions{
			PendingLoginTTL: time.Minute, EnrollmentTTL: time.Minute, RecoveryCodeCount: 10,
			SessionHistoryLimit: 20, SetupTokenTTL: time.Minute,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(context.Background(), Dependencies{
		Auth:         auth,
		AgentModules: catalog,
	})
	if !errors.Is(err, harnessmodule.ErrNoAccessGate) {
		t.Fatalf("New error = %v, want ErrNoAccessGate", err)
	}
}
