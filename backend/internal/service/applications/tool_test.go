package applications

import (
	"context"
	"errors"
	"testing"
)

// recordingInstaller captures the spec the service hands to the container
// layer. What a tool must *not* carry — a device name, a port — is the point.
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

// countingAllocator fails the test by being used at all: a tool has no port to
// allocate, and taking one would reserve a host port for nothing.
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

func toolImage() Image {
	return Image{
		ID:      "mount-tool",
		Name:    "Mount Tool",
		Type:    KindTool,
		Scopes:  []Scope{ScopeProject},
		Service: "mount-tool",
	}
}

func toolService(image Image) (*Service, *recordingInstaller, *countingAllocator, *fakeStore) {
	installer := &recordingInstaller{}
	allocator := &countingAllocator{}
	store := &fakeStore{}
	service := New(
		&singleImageRegistry{image: image},
		store,
		installer,
		&staticProjects{container: "my-project"},
		allocator,
	)
	return service, installer, allocator, store
}

// Installing a tool reaches the project's container and runs its script, but
// claims no host port and names no proxy device.
func TestInstallToolProvisionsWithoutAPort(t *testing.T) {
	service, installer, allocator, _ := toolService(toolImage())

	view, err := service.Install(context.Background(), InstallRequest{
		ImageID:   "mount-tool",
		Scope:     ScopeProject,
		ProjectID: "proj-1",
		Env:       map[string]string{"MOUNT_TOOL_BUCKET": "s3://bucket"},
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if allocator.calls != 0 {
		t.Errorf("allocated %d host ports for a tool; want none", allocator.calls)
	}
	if len(installer.installed) != 1 {
		t.Fatalf("installer calls = %d, want 1", len(installer.installed))
	}
	spec := installer.installed[0]
	if spec.Instance.ContainerName != "my-project" {
		t.Errorf("container = %q, want the project's own", spec.Instance.ContainerName)
	}
	if spec.Instance.DeviceName != "" {
		t.Errorf("device = %q, want none: a tool exposes nothing", spec.Instance.DeviceName)
	}
	if spec.Instance.InternalPort != 0 || spec.Instance.ExternalPort != 0 {
		t.Errorf("ports = %d/%d, want 0/0",
			spec.Instance.InternalPort, spec.Instance.ExternalPort)
	}
	if view.Status != StatusRunning {
		t.Errorf("status = %q, want running", view.Status)
	}
}

// A tool has no port, so asking to change one is a category error rather than
// a silently accepted no-op.
func TestSetPortOnAToolIsRefused(t *testing.T) {
	service, _, _, store := toolService(toolImage())
	store.global = []Instance{{
		ID: "abc123", ImageID: "mount-tool", Scope: ScopeProject,
		ProjectID: "proj-1", Status: StatusRunning,
	}}

	if _, err := service.SetPort(context.Background(), "abc123", 8080); !errors.Is(err, ErrNotSupported) {
		t.Errorf("SetPort on a tool = %v, want ErrNotSupported", err)
	}
}

// A tool is only meaningful inside the container someone works in.
func TestInstallToolRejectsGlobalScope(t *testing.T) {
	service, _, _, _ := toolService(toolImage())

	if _, err := service.Install(context.Background(), InstallRequest{
		ImageID: "mount-tool",
		Scope:   ScopeGlobal,
	}); !errors.Is(err, ErrScope) {
		t.Errorf("global install of a tool = %v, want ErrScope", err)
	}
}
