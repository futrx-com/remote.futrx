package module

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
)

type testProvider struct {
	id agent.ProviderID
}

func (p testProvider) ID() agent.ProviderID { return p.id }

func (p testProvider) Capabilities(context.Context, agent.CapabilityRequest) (agent.Capabilities, error) {
	return agent.Capabilities{Provider: p.id}, nil
}

func (p testProvider) Run(context.Context, agent.RunRequest, func(agent.Event)) error {
	return nil
}

func TestCatalogBuildsFakeFifthAgentWithoutProviderSwitches(t *testing.T) {
	factory := newTestFactory(t, "minimax")
	catalog, err := NewCatalog(factory)
	if err != nil {
		t.Fatal(err)
	}

	runtime, err := catalog.Build(BuildDependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Lookup("minimax") == nil {
		t.Fatal("fake fifth provider was not registered")
	}
	if binding, ok := runtime.AuthBinding("minimax"); !ok || binding.Flow() != agentauth.FlowExternal {
		t.Fatalf("fake fifth auth binding = (%#v, %t)", binding, ok)
	}
	profiles := catalog.Profiles()
	if len(profiles) != 1 || profiles[0].ID != "minimax" {
		t.Fatalf("profiles = %#v", profiles)
	}
}

func TestFactoryBuildReceivesValidatedProfileSnapshot(t *testing.T) {
	descriptor := testDescriptor("future-agent")
	profile := testProfile(descriptor.ID)
	var received *provisioning.Profile
	factory, err := NewFactory(descriptor, &profile, func(_ Dependencies, profile *provisioning.Profile) (Components, error) {
		received = profile
		binding := agentauth.NewExternalBinding(descriptor.ID)
		return Components{Provider: testProvider{id: descriptor.ID}, Auth: &binding}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	profile.CLI.Binary = "changed-after-registration"
	catalog, err := NewCatalog(factory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Build(BuildDependencies{}); err != nil {
		t.Fatal(err)
	}
	if received == nil || received.CLI.Binary != "future-agent" {
		t.Fatalf("build profile = %#v", received)
	}
	received.CLI.Binary = "changed-by-provider"
	if got := catalog.Profiles()[0].CLI.Binary; got != "future-agent" {
		t.Fatalf("provider mutated catalog profile: %q", got)
	}
}

func TestFactoryProjectsApplicationDependenciesBeforeProviderBuild(t *testing.T) {
	descriptor := testDescriptor("future-agent")
	profile := testProfile(descriptor.ID)
	var received Dependencies
	factory, err := NewFactory(
		descriptor,
		&profile,
		func(deps Dependencies, _ *provisioning.Profile) (Components, error) {
			received = deps
			binding := agentauth.NewExternalBinding(descriptor.ID)
			return Components{Provider: testProvider{id: descriptor.ID}, Auth: &binding}, nil
		},
		WithProjectPreparation(ProjectPreparationPolicy{BrowserAssets: true}),
	)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog(factory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Build(BuildDependencies{
		Projects:              moduleTestProjects{},
		CredentialSyncTimeout: 45 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	if received.ProjectPreparer == nil {
		t.Fatal("project provider did not receive the factory-owned preparer")
	}
	if received.CredentialSyncTimeout != 45*time.Second {
		t.Fatalf("credential sync timeout = %s", received.CredentialSyncTimeout)
	}
}

func TestNewFactoryRejectsInvalidProjectPreparationPolicy(t *testing.T) {
	t.Run("nil option", func(t *testing.T) {
		descriptor := testDescriptor("future-agent")
		profile := testProfile(descriptor.ID)
		if _, err := NewFactory(descriptor, &profile, testBuild(descriptor.ID), nil); !errors.Is(err, ErrInvalidFactory) {
			t.Fatalf("NewFactory error = %v, want ErrInvalidFactory", err)
		}
	})

	t.Run("host-only provider", func(t *testing.T) {
		descriptor := testDescriptor("future-agent")
		descriptor.ExecutionScopes = []ExecutionScope{ScopeHost}
		profile := testProfile(descriptor.ID)
		profile.CLI.ImageLabel = ""
		_, err := NewFactory(
			descriptor,
			&profile,
			testBuild(descriptor.ID),
			WithProjectPreparation(ProjectPreparationPolicy{BrowserAssets: true}),
		)
		if !errors.Is(err, ErrInvalidFactory) {
			t.Fatalf("NewFactory error = %v, want ErrInvalidFactory", err)
		}
	})

	t.Run("browser runtime without feature", func(t *testing.T) {
		descriptor := testDescriptor("future-agent")
		profile := testProfile(descriptor.ID)
		_, err := NewFactory(
			descriptor,
			&profile,
			testBuild(descriptor.ID),
			WithProjectPreparation(ProjectPreparationPolicy{BrowserMCPRuntime: true}),
		)
		if !errors.Is(err, ErrInvalidFactory) {
			t.Fatalf("NewFactory error = %v, want ErrInvalidFactory", err)
		}
	})

	t.Run("credential hook without credentials", func(t *testing.T) {
		descriptor := testDescriptor("future-agent")
		profile := testProfile(descriptor.ID)
		_, err := NewFactory(
			descriptor,
			&profile,
			testBuild(descriptor.ID),
			WithProjectPreparation(ProjectPreparationPolicy{
				BeforeCredentials: func(provisioning.Profile) error { return nil },
			}),
		)
		if !errors.Is(err, ErrInvalidFactory) {
			t.Fatalf("NewFactory error = %v, want ErrInvalidFactory", err)
		}
	})
}

func TestNewFactoryRejectsInvalidDeclarations(t *testing.T) {
	valid := testDescriptor("future-agent")
	validProfile := testProfile(valid.ID)
	tests := map[string]Descriptor{
		"empty ID": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.ID = ""
			return descriptor
		}(),
		"unsafe ID": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.ID = "Future Agent"
			return descriptor
		}(),
		"blank label": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.Label = "  "
			return descriptor
		}(),
		"managed auth without instructions": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.Auth = AuthManagedCode
			descriptor.AuthInstructions = ""
			return descriptor
		}(),
		"managed API key without policy": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.Auth = AuthManagedAPIKey
			return descriptor
		}(),
		"managed API key with unsafe URL": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.Auth = AuthManagedAPIKey
			descriptor.APIKeyAuth = &APIKeyAuth{
				CreateURL: "http://example.com/keys", CreateLabel: "Get a key", CredentialLabel: "API key",
			}
			return descriptor
		}(),
		"managed API key without link label": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.Auth = AuthManagedAPIKey
			descriptor.APIKeyAuth = &APIKeyAuth{CreateURL: "https://example.com/keys", CredentialLabel: "API key"}
			return descriptor
		}(),
		"managed API key without credential label": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.Auth = AuthManagedAPIKey
			descriptor.APIKeyAuth = &APIKeyAuth{CreateURL: "https://example.com/keys", CreateLabel: "Get a key"}
			return descriptor
		}(),
		"external auth with API key policy": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.APIKeyAuth = &APIKeyAuth{
				CreateURL: "https://example.com/keys", CreateLabel: "Get a key", CredentialLabel: "API key",
			}
			return descriptor
		}(),
		"external auth gate": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.SatisfiesAccessGate = true
			return descriptor
		}(),
		"project-only default": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.Default = true
			descriptor.ExecutionScopes = []ExecutionScope{ScopeProject}
			return descriptor
		}(),
		"missing scope": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.ExecutionScopes = nil
			return descriptor
		}(),
		"duplicate scope": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.ExecutionScopes = []ExecutionScope{ScopeProject, ScopeProject}
			return descriptor
		}(),
		"fork without resume": func() Descriptor {
			descriptor := cloneDescriptor(valid)
			descriptor.Features.Sessions = SessionSupport{Fork: true}
			return descriptor
		}(),
	}
	for name, descriptor := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := NewFactory(descriptor, &validProfile, testBuild(descriptor.ID)); !errors.Is(err, ErrInvalidFactory) {
				t.Fatalf("NewFactory error = %v, want ErrInvalidFactory", err)
			}
		})
	}
	profileTests := map[string]func(*provisioning.Profile){
		"profile mismatch":                    func(profile *provisioning.Profile) { profile.ID = "other" },
		"incomplete CLI":                      func(profile *provisioning.Profile) { profile.CLI.Binary = "" },
		"invalid CLI version":                 func(profile *provisioning.Profile) { profile.CLI.Version = "latest" },
		"zero CLI install timeout":            func(profile *provisioning.Profile) { profile.CLI.InstallTimeout = 0 },
		"negative CLI wait timeout":           func(profile *provisioning.Profile) { profile.CLI.WaitTimeout = -time.Second },
		"checked CLI without version command": func(profile *provisioning.Profile) { profile.CLI.VersionArgs = nil },
		"project CLI without image label":     func(profile *provisioning.Profile) { profile.CLI.ImageLabel = "" },
		"unsafe persistent host": func(profile *provisioning.Profile) {
			profile.PersistentState = []provisioning.PersistentDirectory{{
				Device: "future-home", HostDirectory: "../future", ContainerPath: "/root/.future",
			}}
		},
		"relative persistent target": func(profile *provisioning.Profile) {
			profile.PersistentState = []provisioning.PersistentDirectory{{
				Device: "future-home", HostDirectory: "future", ContainerPath: "root/.future",
			}}
		},
		"unsafe runtime template target": func(profile *provisioning.Profile) {
			profile.RuntimeAssets = []provisioning.RuntimeAsset{{
				Path: "/root/.future/../escaped", HashPath: "/root/.future/.asset.sha256",
			}}
		},
		"invalid runtime template mode": func(profile *provisioning.Profile) {
			profile.RuntimeAssets = []provisioning.RuntimeAsset{{
				Path: "/root/.future/asset", HashPath: "/root/.future/.asset.sha256", Mode: "u+x",
			}}
		},
	}
	for name, mutate := range profileTests {
		t.Run(name, func(t *testing.T) {
			profile := validProfile.Clone()
			mutate(&profile)
			if _, err := NewFactory(valid, &profile, testBuild(valid.ID)); !errors.Is(err, ErrInvalidFactory) {
				t.Fatalf("NewFactory error = %v, want ErrInvalidFactory", err)
			}
		})
	}
	if _, err := NewFactory(valid, nil, testBuild(valid.ID)); !errors.Is(err, ErrInvalidFactory) {
		t.Fatalf("missing project profile error = %v, want ErrInvalidFactory", err)
	}
	if _, err := NewFactory(valid, &validProfile, nil); !errors.Is(err, ErrInvalidFactory) {
		t.Fatalf("nil builder error = %v, want ErrInvalidFactory", err)
	}
}

func TestCatalogRejectsPersistentStateCollisions(t *testing.T) {
	first := testDescriptor("first-agent")
	firstProfile := testProfile(first.ID)
	firstProfile.PersistentState = []provisioning.PersistentDirectory{{
		Device: "shared-home", HostDirectory: "first", ContainerPath: "/root/.first",
	}}
	second := testDescriptor("second-agent")
	secondProfile := testProfile(second.ID)
	secondProfile.PersistentState = []provisioning.PersistentDirectory{{
		Device: "shared-home", HostDirectory: "second", ContainerPath: "/root/.second",
	}}
	firstFactory, err := NewFactory(first, &firstProfile, testBuild(first.ID))
	if err != nil {
		t.Fatal(err)
	}
	secondFactory, err := NewFactory(second, &secondProfile, testBuild(second.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCatalog(firstFactory, secondFactory); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("NewCatalog error = %v, want ErrInvalidCatalog", err)
	}
}

func TestCatalogRejectsDuplicateFactories(t *testing.T) {
	factory := newTestFactory(t, "future-agent")
	if _, err := NewCatalog(factory, factory); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("NewCatalog error = %v, want ErrInvalidCatalog", err)
	}
}

func TestCatalogRejectsMultipleDefaultProviders(t *testing.T) {
	first := testDescriptor("first-agent")
	first.Default = true
	second := testDescriptor("second-agent")
	second.Default = true
	firstProfile := testProfile(first.ID)
	firstFactory, err := NewFactory(first, &firstProfile, testBuild(first.ID))
	if err != nil {
		t.Fatal(err)
	}
	secondProfile := testProfile(second.ID)
	secondFactory, err := NewFactory(second, &secondProfile, testBuild(second.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCatalog(firstFactory, secondFactory); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("NewCatalog error = %v, want ErrInvalidCatalog", err)
	}
}

func TestFactoryRejectsRuntimeIdentityAndAuthMismatches(t *testing.T) {
	tests := map[string]BuildFunc{
		"provider ID": func(Dependencies, *provisioning.Profile) (Components, error) {
			binding := agentauth.NewExternalBinding("future-agent")
			return Components{Provider: testProvider{id: "other"}, Auth: &binding}, nil
		},
		"auth ID": func(Dependencies, *provisioning.Profile) (Components, error) {
			binding := agentauth.NewExternalBinding("other")
			return Components{Provider: testProvider{id: "future-agent"}, Auth: &binding}, nil
		},
		"auth flow": func(Dependencies, *provisioning.Profile) (Components, error) {
			binding := agentauth.NewCodeBinding("future-agent", agentauth.NewCodeService(agentauth.CodeConfig{}))
			return Components{Provider: testProvider{id: "future-agent"}, Auth: &binding}, nil
		},
		"missing auth": func(Dependencies, *provisioning.Profile) (Components, error) {
			return Components{Provider: testProvider{id: "future-agent"}}, nil
		},
	}
	for name, build := range tests {
		t.Run(name, func(t *testing.T) {
			descriptor := testDescriptor("future-agent")
			profile := testProfile(descriptor.ID)
			factory, err := NewFactory(descriptor, &profile, build)
			if err != nil {
				t.Fatal(err)
			}
			catalog, err := NewCatalog(factory)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := catalog.Build(BuildDependencies{}); !errors.Is(err, ErrInvalidFactory) {
				t.Fatalf("Build error = %v, want ErrInvalidFactory", err)
			}
		})
	}
}

func TestFactoryRejectsUnavailableManagedAuth(t *testing.T) {
	descriptor := testDescriptor("future-agent")
	descriptor.Auth = AuthManagedCode
	descriptor.AuthInstructions = "Complete the code flow."
	profile := testProfile(descriptor.ID)
	factory, err := NewFactory(descriptor, &profile, func(Dependencies, *provisioning.Profile) (Components, error) {
		binding := agentauth.NewCodeBinding("future-agent", nil)
		return Components{Provider: testProvider{id: "future-agent"}, Auth: &binding}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog(factory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Build(BuildDependencies{}); !errors.Is(err, ErrInvalidFactory) {
		t.Fatalf("Build error = %v, want ErrInvalidFactory", err)
	}
}

func TestCatalogSelectsDefaultsAndEvaluatesAccessGate(t *testing.T) {
	authenticated := false
	managed := testDescriptor("managed-agent")
	managed.Default = true
	managed.Auth = AuthManagedDevice
	managed.AuthInstructions = "Complete the device flow."
	managed.SatisfiesAccessGate = true
	managedProfile := testProfile(managed.ID)
	managedFactory, err := NewFactory(managed, &managedProfile, func(Dependencies, *provisioning.Profile) (Components, error) {
		service := agentauth.NewDeviceService(agentauth.DeviceConfig[bindingTestAuthStatus]{
			Authenticated: func() bool { return authenticated },
			BuildStatus: func() agentauth.DeviceStatusBuilder[bindingTestAuthStatus] {
				return func(state agentauth.DeviceState) bindingTestAuthStatus {
					return bindingTestAuthStatus{Authenticated: authenticated, DeviceLogin: state}
				}
			},
		})
		binding := agentauth.NewDeviceBinding(managed.ID, service)
		return Components{Provider: testProvider{id: managed.ID}, Auth: &binding}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	externalFactory := newTestFactory(t, "external-agent")
	catalog, err := NewCatalog(externalFactory, managedFactory)
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.DefaultProvider(ScopeHost); got != managed.ID {
		t.Fatalf("default provider = %q, want %q", got, managed.ID)
	}
	runtime, err := catalog.Build(BuildDependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.AccessReady() {
		t.Fatal("access gate opened before managed authentication")
	}
	authenticated = true
	if !runtime.AccessReady() {
		t.Fatal("access gate stayed closed after managed authentication")
	}

	noAuth := testDescriptor("no-auth-agent")
	noAuth.Auth = AuthNone
	noAuth.AuthInstructions = ""
	noAuth.SatisfiesAccessGate = true
	noAuthProfile := testProfile(noAuth.ID)
	noAuthFactory, err := NewFactory(noAuth, &noAuthProfile, func(Dependencies, *provisioning.Profile) (Components, error) {
		return Components{Provider: testProvider{id: noAuth.ID}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	noAuthCatalog, err := NewCatalog(noAuthFactory)
	if err != nil {
		t.Fatal(err)
	}
	noAuthRuntime, err := noAuthCatalog.Build(BuildDependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if !noAuthRuntime.AccessReady() {
		t.Fatal("no-auth gate provider was not immediately ready")
	}
	if err := noAuthCatalog.ValidateAccessGate(); err != nil {
		t.Fatalf("ValidateAccessGate with no-auth gate = %v", err)
	}
}

func TestCatalogRejectsMissingAccessGateWhenRequested(t *testing.T) {
	catalog, err := NewCatalog(newTestFactory(t, "external-agent"))
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.ValidateAccessGate(); !errors.Is(err, ErrNoAccessGate) {
		t.Fatalf("ValidateAccessGate error = %v, want ErrNoAccessGate", err)
	}
}

type bindingTestAuthStatus struct {
	Authenticated bool                  `json:"authenticated"`
	DeviceLogin   agentauth.DeviceState `json:"deviceLogin"`
}

type moduleTestProjects struct{}

func (moduleTestProjects) Get(context.Context, agent.ProjectID) (agent.Project, error) {
	return agent.Project{}, nil
}

func (moduleTestProjects) Start(context.Context, agent.ProjectID) (agent.Project, error) {
	return agent.Project{}, nil
}

func (moduleTestProjects) ListSecrets(context.Context, agent.ProjectID) ([]agent.ProjectSecret, error) {
	return nil, nil
}

func TestCatalogReturnsDefensiveOrderedSnapshots(t *testing.T) {
	firstDescriptor := testDescriptor("first-agent")
	firstDescriptor.LegacySkillRoots = []string{"/root/.first/skills"}
	firstProfile := testProfile(firstDescriptor.ID)
	firstProfile.Credentials.Files = []provisioning.CredentialFile{{HostPath: "original"}}
	firstProfile.PersistentState = []provisioning.PersistentDirectory{{
		Device: "first-home", HostDirectory: "first", ContainerPath: "/root/.first",
	}}
	firstProfile.RuntimeAssets = []provisioning.RuntimeAsset{{
		Content:  []byte("runtime-original"),
		Path:     "/root/.first/runtime.json",
		HashPath: "/root/.first/.runtime.sha256",
	}}
	firstProfile.BrowserMCPTemplates = []provisioning.TemplateFile{{Content: []byte("original")}}
	firstFactory, err := NewFactory(firstDescriptor, &firstProfile, testBuild(firstDescriptor.ID))
	if err != nil {
		t.Fatal(err)
	}
	secondFactory := newTestFactory(t, "second-agent")
	catalog, err := NewCatalog(firstFactory, secondFactory)
	if err != nil {
		t.Fatal(err)
	}

	firstDescriptor.ExecutionScopes[0] = "changed"
	firstDescriptor.LegacySkillRoots[0] = "/changed"
	firstProfile.Credentials.Files[0].HostPath = "changed"
	firstProfile.PersistentState[0].ContainerPath = "/changed"
	firstProfile.RuntimeAssets[0].Content[0] = 'x'
	firstProfile.BrowserMCPTemplates[0].Content[0] = 'x'

	descriptors := catalog.Descriptors()
	if got := []agent.ProviderID{descriptors[0].ID, descriptors[1].ID}; !slices.Equal(got, []agent.ProviderID{"first-agent", "second-agent"}) {
		t.Fatalf("descriptor order = %v", got)
	}
	descriptors[0].ExecutionScopes[0] = "changed-again"
	descriptors[0].LegacySkillRoots[0] = "/changed-again"

	fresh := catalog.Descriptors()[0]
	if fresh.ExecutionScopes[0] != ScopeHost || fresh.LegacySkillRoots[0] != "/root/.first/skills" {
		t.Fatalf("catalog descriptor mutated through a snapshot: %#v", fresh)
	}
	profiles := catalog.Profiles()
	if profiles[0].Credentials.Files[0].HostPath != "original" ||
		profiles[0].PersistentState[0].ContainerPath != "/root/.first" ||
		string(profiles[0].RuntimeAssets[0].Content) != "runtime-original" ||
		string(profiles[0].BrowserMCPTemplates[0].Content) != "original" {
		t.Fatalf("catalog profile mutated before snapshot: %#v", profiles[0])
	}
	profiles[0].Credentials.Files[0].HostPath = "profile-change"
	profiles[0].RuntimeAssets[0].Content[0] = 'x'
	if got := catalog.Profiles()[0].Credentials.Files[0].HostPath; got != "original" {
		t.Fatalf("catalog profile mutated through a snapshot: %q", got)
	}
	if got := string(catalog.Profiles()[0].RuntimeAssets[0].Content); got != "runtime-original" {
		t.Fatalf("catalog runtime template mutated through a snapshot: %q", got)
	}
}

func TestCatalogEnforcesDeclaredExecutionScopes(t *testing.T) {
	host := testDescriptor("host-agent")
	host.ExecutionScopes = []ExecutionScope{ScopeHost}
	project := testDescriptor("project-agent")
	project.ExecutionScopes = []ExecutionScope{ScopeProject}
	hostFactory, err := NewFactory(host, nil, testBuild(host.ID))
	if err != nil {
		t.Fatal(err)
	}
	projectProfile := testProfile(project.ID)
	projectFactory, err := NewFactory(project, &projectProfile, testBuild(project.ID))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog(hostFactory, projectFactory)
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.SupportsScope("host-agent", ScopeHost) || catalog.SupportsScope("host-agent", ScopeProject) {
		t.Fatal("host-agent scope policy is incorrect")
	}
	if !catalog.SupportsScope("project-agent", ScopeProject) || catalog.SupportsScope("project-agent", ScopeHost) {
		t.Fatal("project-agent scope policy is incorrect")
	}
}

func TestCatalogSeparatesHostAndProjectProvisioningProfiles(t *testing.T) {
	hostOnly := testDescriptor("host-agent")
	hostOnly.ExecutionScopes = []ExecutionScope{ScopeHost}
	hostProfile := testProfile(hostOnly.ID)
	hostProfile.CLI.ImageLabel = ""
	projectOnly := testDescriptor("project-agent")
	projectOnly.ExecutionScopes = []ExecutionScope{ScopeProject}
	projectProfile := testProfile(projectOnly.ID)
	both := testDescriptor("both-agent")
	bothProfile := testProfile(both.ID)
	remoteHost := testDescriptor("remote-host-agent")
	remoteHost.ExecutionScopes = []ExecutionScope{ScopeHost}

	definitions := []struct {
		descriptor Descriptor
		profile    *provisioning.Profile
	}{
		{hostOnly, &hostProfile},
		{projectOnly, &projectProfile},
		{both, &bothProfile},
		{remoteHost, nil},
	}
	factories := make([]Factory, 0, len(definitions))
	for _, definition := range definitions {
		factory, err := NewFactory(definition.descriptor, definition.profile, testBuild(definition.descriptor.ID))
		if err != nil {
			t.Fatalf("NewFactory(%q): %v", definition.descriptor.ID, err)
		}
		factories = append(factories, factory)
	}
	catalog, err := NewCatalog(factories...)
	if err != nil {
		t.Fatal(err)
	}

	projectProfiles := catalog.Profiles()
	if got := []string{projectProfiles[0].ID, projectProfiles[1].ID}; !slices.Equal(got, []string{"project-agent", "both-agent"}) {
		t.Fatalf("project profiles = %v", got)
	}
	hostProfiles := catalog.HostProfiles()
	if got := []string{hostProfiles[0].ID, hostProfiles[1].ID}; !slices.Equal(got, []string{"host-agent", "both-agent"}) {
		t.Fatalf("host profiles = %v", got)
	}

	hostProfiles[0].CLI.Binary = "changed"
	if got := catalog.HostProfiles()[0].CLI.Binary; got != "future-agent" {
		t.Fatalf("host profile mutation escaped catalog: %q", got)
	}
}

func newTestFactory(t *testing.T, id agent.ProviderID) Factory {
	t.Helper()
	descriptor := testDescriptor(id)
	profile := testProfile(id)
	factory, err := NewFactory(descriptor, &profile, testBuild(id))
	if err != nil {
		t.Fatal(err)
	}
	return factory
}

func testBuild(id agent.ProviderID) BuildFunc {
	return func(Dependencies, *provisioning.Profile) (Components, error) {
		binding := agentauth.NewExternalBinding(id)
		return Components{Provider: testProvider{id: id}, Auth: &binding}, nil
	}
}

func testDescriptor(id agent.ProviderID) Descriptor {
	return Descriptor{
		ID:               id,
		Label:            strings.ToUpper(string(id[:1])) + string(id[1:]),
		ExecutionScopes:  []ExecutionScope{ScopeHost, ScopeProject},
		Auth:             AuthExternal,
		AuthInstructions: "Run the provider login command.",
		Features: Features{
			Sessions:       SessionSupport{Resume: true},
			Skills:         SkillsInstructions,
			ScheduledTools: true,
		},
	}
}

func testProfile(id agent.ProviderID) provisioning.Profile {
	return provisioning.Profile{
		ID: string(id),
		CLI: provisioning.CLISpec{
			Name:           "Future Agent",
			ImageLabel:     "future-agent",
			Binary:         "future-agent",
			VersionArgs:    []string{"version"},
			PackageName:    "future-agent-cli",
			Version:        "1.0.0",
			CheckVersion:   true,
			InstallMode:    provisioning.InstallWithNPM,
			InstallTimeout: time.Minute,
			WaitTimeout:    time.Minute,
		},
	}
}
