package applications

import (
	"context"
	"errors"
	"testing"
)

// recordingInstaller captures the spec the service hands to the container
// layer. Portless infrastructure must not carry a device name or port.
type recordingInstaller struct {
	installed []InstallSpec
	stopped   []InstallSpec
	exposed   []InstallSpec
}

func (i *recordingInstaller) Install(_ context.Context, spec InstallSpec) error {
	i.installed = append(i.installed, spec)
	return nil
}
func (i *recordingInstaller) Start(_ context.Context, spec InstallSpec) error {
	i.installed = append(i.installed, spec)
	return nil
}
func (i *recordingInstaller) Stop(_ context.Context, spec InstallSpec) error {
	i.stopped = append(i.stopped, spec)
	return nil
}
func (i *recordingInstaller) Uninstall(_ context.Context, spec InstallSpec) error { return nil }
func (i *recordingInstaller) Expose(_ context.Context, spec InstallSpec) error {
	i.exposed = append(i.exposed, spec)
	return nil
}

// countingAllocator records whether portless infrastructure incorrectly
// reserves a host port for something that exposes nothing.
type countingAllocator struct{ calls int }

func (a *countingAllocator) Allocate(context.Context, string, int, map[int]bool) (int, error) {
	a.calls++
	return 9999, nil
}

type staticProjects struct{ container string }

func (p *staticProjects) ContainerName(context.Context, string) (string, error) {
	return p.container, nil
}
func (p *staticProjects) EnsureRunning(context.Context, string) error { return nil }

func portlessInfrastructureApplication() Application {
	return Application{
		ID:      "mount-tool",
		Name:    "Mount Tool",
		Install: "infra/install.sh",
		Scopes:  []Scope{ScopeProject},
		Service: "mount-tool",
	}
}

func portlessInfrastructureService(application Application) (*Service, *recordingInstaller, *countingAllocator, *fakeStore) {
	installer := &recordingInstaller{}
	allocator := &countingAllocator{}
	store := &fakeStore{}
	service := New(
		&singleApplicationRegistry{application: application},
		store,
		installer,
		&staticProjects{container: "my-project"},
		allocator,
	)
	return service, installer, allocator, store
}

// Installing portless infrastructure reaches the project's container and runs
// its script, but claims no host port and names no proxy device.
func TestInstallPortlessInfrastructureProvisionsWithoutAPort(t *testing.T) {
	service, installer, allocator, _ := portlessInfrastructureService(portlessInfrastructureApplication())

	view, err := service.Install(context.Background(), InstallRequest{
		ApplicationID: "mount-tool",
		Scope:         ScopeProject,
		ProjectID:     "proj-1",
		Env:           map[string]string{"MOUNT_TOOL_BUCKET": "s3://bucket"},
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if allocator.calls != 0 {
		t.Errorf("allocated %d host ports for portless infrastructure; want none", allocator.calls)
	}
	if len(installer.installed) != 1 {
		t.Fatalf("installer calls = %d, want 1", len(installer.installed))
	}
	spec := installer.installed[0]
	if spec.Instance.ContainerName != "my-project" {
		t.Errorf("container = %q, want the project's own", spec.Instance.ContainerName)
	}
	if spec.Instance.DeviceName != "" {
		t.Errorf("device = %q, want none: the application exposes nothing", spec.Instance.DeviceName)
	}
	if spec.Instance.InternalPort != 0 || spec.Instance.ExternalPort != 0 {
		t.Errorf("ports = %d/%d, want 0/0",
			spec.Instance.InternalPort, spec.Instance.ExternalPort)
	}
	if view.Status != StatusRunning {
		t.Errorf("status = %q, want running", view.Status)
	}
}

// An application without a port rejects a port change rather than silently
// accepting a no-op.
func TestSetPortOnPortlessInfrastructureIsRefused(t *testing.T) {
	service, _, _, store := portlessInfrastructureService(portlessInfrastructureApplication())
	store.global = []Instance{{
		ID: "abc123", ApplicationID: "mount-tool", Scope: ScopeProject,
		ProjectID: "proj-1", Status: StatusRunning,
	}}

	if _, err := service.SetPort(context.Background(), "abc123", 8080); !errors.Is(err, ErrNotSupported) {
		t.Errorf("SetPort on portless infrastructure = %v, want ErrNotSupported", err)
	}
}

// Capability inference does not override the scopes declared by an application.
func TestInstallPortlessInfrastructureRespectsDeclaredScopes(t *testing.T) {
	service, _, _, _ := portlessInfrastructureService(portlessInfrastructureApplication())

	if _, err := service.Install(context.Background(), InstallRequest{
		ApplicationID: "mount-tool",
		Scope:         ScopeGlobal,
	}); !errors.Is(err, ErrScope) {
		t.Errorf("global install outside the declared scopes = %v, want ErrScope", err)
	}
}
