package applications

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// fakeRunner records the lxc invocations an installer makes, so a test can
// assert on what it did (and, more importantly, on what it did not do).
type fakeRunner struct {
	// running is the set of containers `lxc info` should report as up.
	running map[string]bool
	calls   [][]string
}

func newFakeRunner(running ...string) *fakeRunner {
	set := map[string]bool{}
	for _, name := range running {
		set[name] = true
	}
	return &fakeRunner{running: set}
}

func (f *fakeRunner) Available() bool { return true }

func (f *fakeRunner) Run(_ context.Context, args ...string) (string, error) {
	f.calls = append(f.calls, args)
	switch {
	case len(args) >= 2 && args[0] == "info":
		if f.running[args[1]] {
			return "Name: " + args[1] + "\nStatus: RUNNING\n", nil
		}
		return "Error: Instance not found", fmt.Errorf("exit status 1")
	case len(args) >= 1 && args[0] == "exec":
		// waitNetwork probes for connectivity; report it immediately.
		return "ok", nil
	}
	return "", nil
}

func (f *fakeRunner) RunStdin(_ context.Context, stdin io.Reader, args ...string) (string, error) {
	f.calls = append(f.calls, args)
	if stdin != nil {
		_, _ = io.ReadAll(stdin)
	}
	return "", nil
}

// commands returns each recorded invocation as one space-joined string.
func (f *fakeRunner) commands() []string {
	out := make([]string, 0, len(f.calls))
	for _, call := range f.calls {
		out = append(out, strings.Join(call, " "))
	}
	return out
}

func (f *fakeRunner) any(predicate func(string) bool) bool {
	for _, cmd := range f.commands() {
		if predicate(cmd) {
			return true
		}
	}
	return false
}

func (f *fakeRunner) hasPrefix(prefix string) bool {
	return f.any(func(cmd string) bool { return strings.HasPrefix(cmd, prefix) })
}

func (f *fakeRunner) contains(fragment string) bool {
	return f.any(func(cmd string) bool { return strings.Contains(cmd, fragment) })
}

func testInstaller(t *testing.T, runner *fakeRunner) *Installer {
	t.Helper()
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	return NewInstaller(runner, registry)
}

// mysql is used throughout because it is a real service image with an install
// script the registry can hand the installer.
func serviceSpec(scope svc.Scope, container string) svc.InstallSpec {
	return svc.InstallSpec{
		Image: svc.Image{
			ID:      "mysql",
			Name:    "MySQL",
			Type:    svc.KindService,
			Service: "mysql",
			Port:    svc.Port{Internal: 3306, DefaultExternal: 3306},
		},
		Instance: svc.Instance{
			ID:            "abc123",
			ImageID:       "mysql",
			Scope:         scope,
			ContainerName: container,
			DeviceName:    "app-abc123",
			InternalPort:  3306,
			ExternalPort:  3307,
			BindAddress:   "127.0.0.1",
			Protocol:      svc.ProtocolTCP,
		},
	}
}

// A project-scope service installs into the project's own container. Launching
// a second container for it would double the memory cost of every app and put
// it off the project's filesystem, so this asserts no container is created.
func TestInstallProjectScopeUsesTheProjectContainer(t *testing.T) {
	runner := newFakeRunner("my-project")
	installer := testInstaller(t, runner)

	if err := installer.Install(context.Background(), serviceSpec(svc.ScopeProject, "my-project")); err != nil {
		t.Fatalf("install: %v", err)
	}

	for _, forbidden := range []string{"launch", "init", "copy", "create"} {
		if runner.hasPrefix(forbidden + " ") {
			t.Errorf("project install ran %q; it must reuse the project container:\n%s",
				forbidden, strings.Join(runner.commands(), "\n"))
		}
	}
	if !runner.contains("exec my-project") {
		t.Errorf("want the install script to run inside my-project, got:\n%s",
			strings.Join(runner.commands(), "\n"))
	}
	if !runner.contains("bash -s") {
		t.Error("want the install script to be piped into bash -s")
	}
	if !runner.hasPrefix("config device add my-project app-abc123 proxy") {
		t.Errorf("want a proxy device on the project container, got:\n%s",
			strings.Join(runner.commands(), "\n"))
	}
}

// A global service has no project container to live in, so it gets its own.
func TestInstallGlobalScopeLaunchesADedicatedContainer(t *testing.T) {
	runner := newFakeRunner() // nothing running: the container does not exist yet
	installer := testInstaller(t, runner)
	spec := serviceSpec(svc.ScopeGlobal, "futrx-app-abc123")

	if err := installer.Install(context.Background(), spec); err != nil {
		t.Fatalf("install: %v", err)
	}

	if !runner.hasPrefix("launch " + defaultBase + " futrx-app-abc123") {
		t.Errorf("want a dedicated container launched from %s, got:\n%s",
			defaultBase, strings.Join(runner.commands(), "\n"))
	}
	if !runner.contains("exec futrx-app-abc123") {
		t.Error("want the install script to run inside the dedicated container")
	}
	if !runner.hasPrefix("config device add futrx-app-abc123 app-abc123 proxy") {
		t.Error("want a proxy device on the dedicated container")
	}
}

func TestInstallGlobalScopeHonoursTheImageBase(t *testing.T) {
	runner := newFakeRunner()
	installer := testInstaller(t, runner)
	spec := serviceSpec(svc.ScopeGlobal, "futrx-app-abc123")
	spec.Image.Base = "images:debian/12"

	if err := installer.Install(context.Background(), spec); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !runner.hasPrefix("launch images:debian/12 futrx-app-abc123") {
		t.Errorf("want the image's own base used, got:\n%s", strings.Join(runner.commands(), "\n"))
	}
}

// Reinstall/start re-runs the script; an already-running container must not be
// launched a second time.
func TestInstallGlobalScopeReusesARunningContainer(t *testing.T) {
	runner := newFakeRunner("futrx-app-abc123")
	installer := testInstaller(t, runner)

	if err := installer.Install(context.Background(), serviceSpec(svc.ScopeGlobal, "futrx-app-abc123")); err != nil {
		t.Fatalf("install: %v", err)
	}
	if runner.hasPrefix("launch ") {
		t.Errorf("want the running container reused, got:\n%s", strings.Join(runner.commands(), "\n"))
	}
}

// Stopping a project app must not stop the project's container — the user is
// probably still working in it.
func TestStopProjectScopeStopsOnlyTheService(t *testing.T) {
	runner := newFakeRunner("my-project")
	installer := testInstaller(t, runner)

	if err := installer.Stop(context.Background(), serviceSpec(svc.ScopeProject, "my-project")); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if runner.hasPrefix("stop ") {
		t.Errorf("project stop must not stop the container, got:\n%s",
			strings.Join(runner.commands(), "\n"))
	}
	if !runner.contains("systemctl stop mysql") {
		t.Errorf("want the systemd unit stopped, got:\n%s", strings.Join(runner.commands(), "\n"))
	}
}

func TestStopGlobalScopeStopsTheDedicatedContainer(t *testing.T) {
	runner := newFakeRunner("futrx-app-abc123")
	installer := testInstaller(t, runner)

	if err := installer.Stop(context.Background(), serviceSpec(svc.ScopeGlobal, "futrx-app-abc123")); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !runner.hasPrefix("stop --force futrx-app-abc123") {
		t.Errorf("want the dedicated container stopped, got:\n%s",
			strings.Join(runner.commands(), "\n"))
	}
}

// Uninstalling a project app must never delete the project's container.
func TestUninstallProjectScopeKeepsTheProjectContainer(t *testing.T) {
	runner := newFakeRunner("my-project")
	installer := testInstaller(t, runner)

	if err := installer.Uninstall(context.Background(), serviceSpec(svc.ScopeProject, "my-project")); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if runner.hasPrefix("delete ") {
		t.Fatalf("project uninstall must not delete the container, got:\n%s",
			strings.Join(runner.commands(), "\n"))
	}
	if !runner.hasPrefix("config device remove my-project app-abc123") {
		t.Error("want the proxy device removed")
	}
	if !runner.contains("systemctl disable --now mysql") {
		t.Error("want the systemd unit disabled")
	}
}

func TestUninstallGlobalScopeDeletesTheDedicatedContainer(t *testing.T) {
	runner := newFakeRunner("futrx-app-abc123")
	installer := testInstaller(t, runner)

	if err := installer.Uninstall(context.Background(), serviceSpec(svc.ScopeGlobal, "futrx-app-abc123")); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !runner.hasPrefix("delete --force futrx-app-abc123") {
		t.Errorf("want the dedicated container deleted, got:\n%s",
			strings.Join(runner.commands(), "\n"))
	}
}
