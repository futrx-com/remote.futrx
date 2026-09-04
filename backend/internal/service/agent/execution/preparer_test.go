package execution

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

func TestPreparerAppliesSharedWorkflowInOrder(t *testing.T) {
	recorder := &preparationRecorder{}
	profile := preparationTestProfile()
	preparer := New(
		preparationProjects{recorder: recorder},
		preparationDependencies(recorder),
		Options{
			Provider:          "future-agent",
			Profile:           profile,
			BrowserAssets:     true,
			BrowserMCPRuntime: true,
			BeforeCredentials: func(provisioning.Profile) error {
				recorder.calls = append(recorder.calls, "before-credentials")
				return nil
			},
		},
	)
	profile.CLI.Binary = "mutated-after-construction"

	var events []string
	prepared, err := preparer.Prepare(context.Background(), agent.ProjectPreparationRequest{
		ProjectID:           "project-id",
		ConversationID:      "conversation-id",
		EnableBrowser:       true,
		EnableScheduleTools: true,
	}, func(event agent.Event) {
		events = append(events, string(event.Provider)+":"+event.ConversationID+":"+event.Subtype)
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{
		"get", "start", "cli:future", "before-credentials", "credentials",
		"instructions", "runtime-assets", "skill-links", "browser-skill", "browser-script",
		"browser-mcp", "browser-core", "schedule", "lifecycle", "secrets",
	}
	if !slices.Equal(recorder.calls, wantCalls) {
		t.Fatalf("preparation calls\n got: %v\nwant: %v", recorder.calls, wantCalls)
	}
	if !slices.Equal(events, []string{
		"future-agent:conversation-id:container_starting",
		"future-agent:conversation-id:container_preparing",
	}) {
		t.Fatalf("events = %v", events)
	}
	if prepared.ID != "project-id" || prepared.ContainerName != "project-container" ||
		len(prepared.Secrets) != 1 || prepared.Secrets[0].Key != "PROJECT_SECRET" {
		t.Fatalf("prepared project = %#v", prepared)
	}
	if recorder.runtimeAssetContainer != "project-container" || len(recorder.runtimeAssets) != 1 ||
		recorder.runtimeAssets[0].Path != "/root/.future/runtime.json" ||
		string(recorder.runtimeAssets[0].Content) != "runtime-template" {
		t.Fatalf("runtime asset request = container %q, templates %#v",
			recorder.runtimeAssetContainer, recorder.runtimeAssets)
	}
}

func TestPreparerPreservesStrictSkillLinkPolicy(t *testing.T) {
	recorder := &preparationRecorder{skillLinkError: errors.New("link failed")}
	preparer := New(
		preparationProjects{recorder: recorder},
		preparationDependencies(recorder),
		Options{
			Provider:           "future-agent",
			Profile:            preparationTestProfile(),
			SkillLinksRequired: true,
		},
	)
	_, err := preparer.Prepare(
		context.Background(),
		agent.ProjectPreparationRequest{ProjectID: "project-id"},
		nil,
	)
	if err == nil || err.Error() != "prepare workspace skill links: link failed" {
		t.Fatalf("Prepare error = %v", err)
	}
}

func TestPreparerRuntimeAssetFailurePreservesErrorAndShortCircuits(t *testing.T) {
	recorder := &preparationRecorder{runtimeAssetError: errors.New("publish failed")}
	preparer := New(
		preparationProjects{recorder: recorder},
		preparationDependencies(recorder),
		Options{Provider: "future-agent", Profile: preparationTestProfile()},
	)

	_, err := preparer.Prepare(
		context.Background(),
		agent.ProjectPreparationRequest{ProjectID: "project-id"},
		nil,
	)
	if err == nil || err.Error() != "push agent runtime assets to container: publish failed" {
		t.Fatalf("Prepare error = %v", err)
	}
	wantCalls := []string{
		"get", "start", "cli:future", "credentials", "instructions", "runtime-assets",
	}
	if !slices.Equal(recorder.calls, wantCalls) {
		t.Fatalf("preparation calls\n got: %v\nwant: %v", recorder.calls, wantCalls)
	}
}

type preparationRecorder struct {
	calls                 []string
	skillLinkError        error
	runtimeAssetError     error
	runtimeAssetContainer string
	runtimeAssets         []provisioning.RuntimeAsset
}

type preparationProjects struct{ recorder *preparationRecorder }

func (p preparationProjects) Get(context.Context, agent.ProjectID) (agent.Project, error) {
	p.recorder.calls = append(p.recorder.calls, "get")
	return agent.Project{ID: "project-id", ContainerName: "project-container", Status: "stopped"}, nil
}

func (p preparationProjects) Start(context.Context, agent.ProjectID) (agent.Project, error) {
	p.recorder.calls = append(p.recorder.calls, "start")
	return agent.Project{}, nil
}

func (p preparationProjects) ListSecrets(context.Context, agent.ProjectID) ([]agent.ProjectSecret, error) {
	p.recorder.calls = append(p.recorder.calls, "secrets")
	return []agent.ProjectSecret{{Key: "PROJECT_SECRET", Value: "value"}}, nil
}

type preparationCLI struct{ recorder *preparationRecorder }

func (p preparationCLI) Ensure(_ context.Context, _ string, spec provisioning.CLISpec) error {
	p.recorder.calls = append(p.recorder.calls, "cli:"+spec.Binary)
	return nil
}

type preparationCredentials struct{ recorder *preparationRecorder }

func (p preparationCredentials) Ensure(context.Context, string, provisioning.CredentialSpec) error {
	p.recorder.calls = append(p.recorder.calls, "credentials")
	return nil
}

func (p preparationCredentials) SyncFromContainer(context.Context, string, provisioning.CredentialSpec) error {
	return nil
}

type preparationWorkspace struct{ recorder *preparationRecorder }

func (p preparationWorkspace) EnsureAgentInstructions(context.Context, string) error {
	p.recorder.calls = append(p.recorder.calls, "instructions")
	return nil
}

func (p preparationWorkspace) EnsureSkillLinks(context.Context, string) error {
	p.recorder.calls = append(p.recorder.calls, "skill-links")
	return p.recorder.skillLinkError
}

type preparationRuntimeAssets struct{ recorder *preparationRecorder }

func (p preparationRuntimeAssets) Ensure(
	_ context.Context,
	containerName string,
	assets []provisioning.RuntimeAsset,
) error {
	p.recorder.calls = append(p.recorder.calls, "runtime-assets")
	p.recorder.runtimeAssetContainer = containerName
	p.recorder.runtimeAssets = assets
	return p.recorder.runtimeAssetError
}

type preparationBrowser struct{ recorder *preparationRecorder }

func (p preparationBrowser) EnsureSkill(context.Context, string) error {
	p.recorder.calls = append(p.recorder.calls, "browser-skill")
	return nil
}

func (p preparationBrowser) EnsureScript(context.Context, string) error {
	p.recorder.calls = append(p.recorder.calls, "browser-script")
	return nil
}

func (p preparationBrowser) EnsureMCP(context.Context, string) error {
	p.recorder.calls = append(p.recorder.calls, "browser-mcp")
	return nil
}

func (p preparationBrowser) EnsureCore(context.Context, string) error {
	p.recorder.calls = append(p.recorder.calls, "browser-core")
	return nil
}

type preparationSchedule struct{ recorder *preparationRecorder }

func (p preparationSchedule) Ensure(context.Context, string) error {
	p.recorder.calls = append(p.recorder.calls, "schedule")
	return nil
}

type preparationLifecycle struct{ recorder *preparationRecorder }

func (p preparationLifecycle) EnsureBootAutostart(context.Context, string) error {
	p.recorder.calls = append(p.recorder.calls, "lifecycle")
	return nil
}

func preparationDependencies(recorder *preparationRecorder) provisioning.ContainerDependencies {
	return provisioning.ContainerDependencies{
		CLI:           preparationCLI{recorder},
		Credentials:   preparationCredentials{recorder},
		Workspace:     preparationWorkspace{recorder},
		RuntimeAssets: preparationRuntimeAssets{recorder},
		Browser:       preparationBrowser{recorder},
		ScheduleTools: preparationSchedule{recorder},
		Lifecycle:     preparationLifecycle{recorder},
	}
}

func preparationTestProfile() provisioning.Profile {
	return provisioning.Profile{
		ID:  "future-agent",
		CLI: provisioning.CLISpec{Binary: "future"},
		Credentials: provisioning.CredentialSpec{Files: []provisioning.CredentialFile{{
			HostPath: "/host/credentials", ContainerPath: "/root/.future/credentials",
		}}},
		RuntimeAssets: []provisioning.RuntimeAsset{{
			Content:  []byte("runtime-template"),
			Path:     "/root/.future/runtime.json",
			HashPath: "/root/.future/.runtime.sha256",
		}},
	}
}
