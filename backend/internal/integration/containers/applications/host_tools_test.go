package applications

import (
	"context"
	"errors"
	"reflect"
	"testing"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

type fakeHostTools struct {
	calls int
	got   []svc.HostTool
	err   error
}

func (f *fakeHostTools) Ensure(_ context.Context, tools []svc.HostTool) error {
	f.calls++
	f.got = tools
	return f.err
}

// toolSpecWithHostTool is the fixture tool as the registry loads it, so the
// declaration under test is the one an image actually ships.
func toolSpecWithHostTool(t *testing.T, in *Installer, env map[string]string) svc.InstallSpec {
	t.Helper()
	img, ok := in.registry.Get(fixtureTool)
	if !ok {
		t.Fatal("missing the fixture tool image")
	}
	return svc.InstallSpec{
		Image:    img,
		Instance: svc.Instance{ImageID: img.ID, Scope: svc.ScopeProject, ContainerName: "project", Env: env},
	}
}

// A host tool is a precondition, not a side effect: the guest installer must
// not run until it is present, and it must be repaired on every start.
func TestInstallAndStartPrepareDeclaredHostTools(t *testing.T) {
	runner := newFakeRunner("project")
	in := testInstaller(t, runner)
	tools := &fakeHostTools{}
	in.hostTools = tools
	spec := toolSpecWithHostTool(t, in, nil)

	tools.err = errors.New("host install failed")
	if err := in.Install(context.Background(), spec); err == nil {
		t.Fatal("ignored a host tool failure")
	}
	if len(runner.calls) != 0 {
		t.Fatal("ran the guest installer after the host tool failed")
	}

	tools.err = nil
	if err := in.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if tools.calls != 2 {
		t.Fatalf("host tool ensured %d times, want start to repair it too", tools.calls)
	}
	if want := spec.Image.HostTools; !reflect.DeepEqual(tools.got, want) {
		t.Errorf("ensured %+v, want the image's own declaration %+v", tools.got, want)
	}
	// The host tool is the server's dependency, not the container's: nothing
	// about it may be installed through lxc.
	if runner.contains("apt-get") || runner.contains("fixture-backup") {
		t.Error("host tool commands ran through LXC")
	}
}

// An image with no host tools must not reach the tool installer at all, so a
// Remote host that installs only ordinary images downloads nothing.
func TestInstallSkipsHostToolsWhenNoneAreDeclared(t *testing.T) {
	runner := newFakeRunner("my-project")
	in := testInstaller(t, runner)
	tools := &fakeHostTools{err: errors.New("must not be called")}
	in.hostTools = tools

	if err := in.Install(context.Background(), serviceSpec(svc.ScopeProject, "my-project")); err != nil {
		t.Fatal(err)
	}
	if tools.calls != 0 {
		t.Errorf("ensured host tools %d times for an image that declares none", tools.calls)
	}
}

// The host learns the backup destination through the mapping the image
// declares, under the image's own variable names — the installer resolves it,
// so nothing in Remote has to know what those names are.

// A half-declared mapping produces a destination the store cannot use, so it is
// rejected when the catalog loads rather than on someone's host.
func TestRegistryRejectsIncompleteImageDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		image svc.Image
	}{
		{
			name: "a host tool without a checksum",
			image: svc.Image{Name: "Test", Type: svc.KindTool, Scopes: []svc.Scope{svc.ScopeProject}, HostTools: []svc.HostTool{{
				Name:      "tool",
				Version:   "1",
				Downloads: map[string]svc.HostToolDownload{"amd64": {URL: "https://example.invalid/tool"}},
			}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validate(tc.image); err == nil {
				t.Error("want a validation error, got nil")
			}
		})
	}
}
