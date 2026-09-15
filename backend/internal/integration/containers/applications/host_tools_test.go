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

// hostToolSpec is the portless fixture as the registry loads it, so the
// declaration under test is the one an application actually ships.
func hostToolSpec(t *testing.T, in *Installer, env map[string]string) svc.InstallSpec {
	t.Helper()
	application, ok := in.registry.Get(fixturePortless)
	if !ok {
		t.Fatal("missing the portless fixture application")
	}
	return svc.InstallSpec{
		Application: application,
		Instance:    svc.Instance{ApplicationID: application.ID, Scope: svc.ScopeProject, ContainerName: "project", Env: env},
	}
}

// A host tool is a precondition, not a side effect: the guest installer must
// not run until it is present, and it must be repaired on every start.
func TestInstallAndStartPrepareDeclaredHostTools(t *testing.T) {
	runner := newFakeRunner("project")
	in := testInstaller(t, runner)
	tools := &fakeHostTools{}
	in.hostTools = tools
	spec := hostToolSpec(t, in, nil)

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
	if want := spec.Application.HostTools; !reflect.DeepEqual(tools.got, want) {
		t.Errorf("ensured %+v, want the application's own declaration %+v", tools.got, want)
	}
	// The host tool is the server's dependency, not the container's: nothing
	// about it may be installed through lxc.
	if runner.contains("apt-get") || runner.contains("fixture-backup") {
		t.Error("host tool commands ran through LXC")
	}
}

// An application with no host tools must not reach the tool installer at all, so a
// Remote host that installs only ordinary applications downloads nothing.
func TestInstallSkipsHostToolsWhenNoneAreDeclared(t *testing.T) {
	runner := newFakeRunner("my-project")
	in := testInstaller(t, runner)
	tools := &fakeHostTools{err: errors.New("must not be called")}
	in.hostTools = tools

	if err := in.Install(context.Background(), serviceSpec(svc.ScopeProject, "my-project")); err != nil {
		t.Fatal(err)
	}
	if tools.calls != 0 {
		t.Errorf("ensured host tools %d times for an application that declares none", tools.calls)
	}
}

// The host learns the backup destination through the mapping the application
// declares, under the application's own variable names — the installer resolves it,
// so nothing in Remote has to know what those names are.

// A half-declared mapping produces a destination the store cannot use, so it is
// rejected when the catalog loads rather than on someone's host.
func TestRegistryRejectsIncompleteApplicationDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name        string
		application svc.Application
	}{
		{
			name: "a host tool without a checksum",
			application: svc.Application{Name: "Test", Install: "infra/install.sh", Scopes: []svc.Scope{svc.ScopeProject}, HostTools: []svc.HostTool{{
				Name:      "tool",
				Version:   "1",
				Downloads: map[string]svc.HostToolDownload{"amd64": {URL: "https://example.invalid/tool"}},
			}}},
		},
		{
			name: "a host tool on an application that provisions nothing",
			application: svc.Application{Name: "Test", Scopes: []svc.Scope{svc.ScopeProject}, HostTools: []svc.HostTool{{
				Name:      "tool",
				Version:   "1",
				Downloads: map[string]svc.HostToolDownload{"amd64": {URL: "https://example.invalid/tool", SHA256: "00"}},
			}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateApplication(tc.application); err == nil {
				t.Error("want a validation error, got nil")
			}
		})
	}
}
