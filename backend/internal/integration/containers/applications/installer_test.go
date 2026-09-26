package applications

import (
	"context"
	"encoding/base64"
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
	running     map[string]bool
	missingUnit bool
	calls       [][]string
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
		if f.missingUnit && strings.Contains(strings.Join(args, " "), "test -f /etc/systemd/system/fixture.service") {
			return "", fmt.Errorf("exit status 1")
		}
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
	return NewInstaller(runner, testRegistry(t), t.TempDir())
}

func TestContainerStateRejectsAnEmptyNameWithoutCallingLXD(t *testing.T) {
	runner := newFakeRunner()
	installer := testInstaller(t, runner)

	if _, err := installer.containerState(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "container name is required") {
		t.Fatalf("error = %v, want a missing container name error", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("empty name reached LXD: %v", runner.calls)
	}
}

// The fixture service application is used throughout: it has an install script the
// registry can hand the installer, and a port to proxy.
func serviceSpec(scope svc.Scope, container string) svc.InstallSpec {
	return svc.InstallSpec{
		Application: svc.Application{
			ID:      fixtureService,
			Name:    "Fixture Service",
			Install: "infra/install.sh",
			Service: &svc.ApplicationService{
				Name: "fixture", Command: []string{"/usr/local/bin/fixture", "--port", "{{internalPort}}"},
			},
			Port: svc.Port{Internal: 3306, DefaultExternal: 3306},
		},
		Instance: svc.Instance{
			ID:            "abc123",
			ApplicationID: fixtureService,
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

func TestManifestServiceRendersStandardUnitAndEncodedEnvironment(t *testing.T) {
	spec := serviceSpec(svc.ScopeProject, "my-project")
	spec.Application.Service = &svc.ApplicationService{
		Name:        "fixture",
		Description: "Fixture Service",
		Command:     []string{"/usr/local/bin/fixture", "serve", "--port", "{{internalPort}}"},
		User:        "nobody",
		Group:       "nogroup",
		Restart:     "on-failure",
		RestartSec:  2,
		Environment: []svc.ServiceEnvironment{
			{Key: "FIXTURE_SECRET_B64", FromEnv: "FIXTURE_SECRET", Encoding: "base64"},
		},
		Hardening: svc.ServiceHardening{
			NoNewPrivileges: true,
			PrivateTmp:      true,
			ProtectHome:     true,
			ProtectSystem:   "strict",
		},
	}
	spec.Instance.Env = map[string]string{"FIXTURE_SECRET": "line one\nline two='$value'"}

	environment := string(serviceEnvironment(spec))
	wantEnvironment := "FIXTURE_SECRET_B64=" + base64.StdEncoding.EncodeToString([]byte(spec.Instance.Env["FIXTURE_SECRET"])) + "\n"
	if environment != wantEnvironment {
		t.Fatalf("environment = %q, want %q", environment, wantEnvironment)
	}

	unit := string(serviceUnit(spec, "/etc/remote/applications/fixture/environment"))
	for _, fragment := range []string{
		"Description=Fixture Service",
		"EnvironmentFile=/etc/remote/applications/fixture/environment",
		`ExecStart="/usr/local/bin/fixture" "serve" "--port" "3306"`,
		"Restart=on-failure",
		"RestartSec=2s",
		"User=nobody",
		"Group=nogroup",
		"NoNewPrivileges=true",
		"PrivateTmp=true",
		"ProtectHome=true",
		"ProtectSystem=strict",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, fragment) {
			t.Errorf("unit does not contain %q:\n%s", fragment, unit)
		}
	}
}

func TestInstallMaterializesAndStartsTheManifestService(t *testing.T) {
	runner := newFakeRunner("my-project")
	installer := testInstaller(t, runner)

	if err := installer.Install(context.Background(), serviceSpec(svc.ScopeProject, "my-project")); err != nil {
		t.Fatalf("install: %v", err)
	}

	for _, command := range []string{
		"file push --mode=600",
		"file push --mode=644",
		"systemctl daemon-reload",
		"systemctl enable fixture",
		"systemctl restart fixture",
	} {
		if !runner.contains(command) {
			t.Errorf("install did not run %q:\n%s", command, strings.Join(runner.commands(), "\n"))
		}
	}
}

func TestSocketProxyStartsOnDemandAndStopsWithTheApplication(t *testing.T) {
	runner := newFakeRunner("my-project")
	installer := testInstaller(t, runner)
	spec := serviceSpec(svc.ScopeProject, "my-project")
	spec.Application.Service.SocketProxy = &svc.SocketProxy{
		ListenPort: 8842, TargetPort: 8081, IdleSeconds: 600, ReadyPath: "/healthz",
	}
	spec.Application.Port = svc.Port{}
	spec.Instance.DeviceName = ""
	if err := installer.Install(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if !runner.contains("systemctl enable --now fixture.socket") || runner.contains("systemctl restart fixture") {
		t.Fatalf("install should arm only the socket: %v", runner.commands())
	}
	unit := string(serviceUnit(spec, "/etc/remote/applications/fixture/environment"))
	if !strings.Contains(unit, "StopWhenUnneeded=yes") || !strings.Contains(unit, "ExecStartPost=") {
		t.Fatalf("service does not stop when idle or wait for readiness: %s", unit)
	}
	if !strings.Contains(string(socketUnit("fixture", *spec.Application.Service.SocketProxy)), "ListenStream=0.0.0.0:8842") ||
		!strings.Contains(string(socketProxyUnit("fixture", *spec.Application.Service.SocketProxy)), "--exit-idle-time=600s 127.0.0.1:8081") {
		t.Fatal("socket proxy does not preserve the declared endpoints")
	}
	runner.calls = nil
	if err := installer.Stop(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if !runner.contains("systemctl disable --now fixture.socket") || !runner.contains("systemctl stop fixture-proxy.service") || !runner.contains("systemctl stop fixture") {
		t.Fatalf("stop left a socket or process active: %v", runner.commands())
	}
	runner.calls = nil
	if err := installer.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if !runner.contains("systemctl enable --now fixture.socket") || runner.contains("systemctl start fixture") {
		t.Fatalf("start should re-arm the socket without starting the editor: %v", runner.commands())
	}
}

func TestStartReinstallsStoppedServiceAfterProjectContainerReplacement(t *testing.T) {
	runner := newFakeRunner("my-project")
	runner.missingUnit = true
	installer := testInstaller(t, runner)
	spec := serviceSpec(svc.ScopeProject, "my-project")
	if err := installer.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if !runner.contains("bash -s") || !runner.contains("systemctl restart fixture") {
		t.Fatalf("missing service was not reinstalled: %v", runner.commands())
	}
}

func TestInstallSupportsAServiceWithNoCustomInstaller(t *testing.T) {
	runner := newFakeRunner("my-project")
	installer := testInstaller(t, runner)
	spec := serviceSpec(svc.ScopeProject, "my-project")
	spec.Application.Install = ""
	spec.Application.Container = nil

	if err := installer.Install(context.Background(), spec); err != nil {
		t.Fatalf("install: %v", err)
	}
	if runner.contains("bash -s") {
		t.Errorf("service-only application ran a custom installer:\n%s", strings.Join(runner.commands(), "\n"))
	}
	if !runner.contains("systemctl restart fixture") {
		t.Errorf("manifest service was not started:\n%s", strings.Join(runner.commands(), "\n"))
	}
}

func TestInstallScriptEnvironmentCarriesManifestMetadata(t *testing.T) {
	spec := serviceSpec(svc.ScopeProject, "my-project")
	spec.Application.Version = "4.5.6"
	spec.Instance.Env = map[string]string{
		"CUSTOM":             "value",
		"APP_APPLICATION_ID": "forged",
	}
	env := testInstaller(t, newFakeRunner()).scriptEnv(spec)
	for key, want := range map[string]string{
		"APP_APPLICATION_ID":      fixtureService,
		"APP_APPLICATION_NAME":    "Fixture Service",
		"APP_APPLICATION_VERSION": "4.5.6",
		"APP_SERVICE":             "fixture",
		"APP_INTERNAL_PORT":       "3306",
		"CUSTOM":                  "value",
	} {
		if env[key] != want {
			t.Errorf("%s = %q, want %q", key, env[key], want)
		}
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

// portlessInfrastructureSpec describes a real container and install script,
// but no device name, ports, or proxy.
func portlessInfrastructureSpec(container string) svc.InstallSpec {
	return svc.InstallSpec{
		Application: svc.Application{
			ID:      fixturePortless,
			Name:    "Fixture Portless",
			Install: "infra/install.sh",
			Service: &svc.ApplicationService{
				Name: "fixture-portless", Command: []string{"/usr/local/bin/fixture-portless"},
			},
		},
		Instance: svc.Instance{
			ID:            "abc123",
			ApplicationID: fixturePortless,
			Scope:         svc.ScopeProject,
			ContainerName: container,
		},
	}
}

// Portless infrastructure provisions software into the project container but
// exposes nothing. Creating a proxy would claim a host port for no listener.
func TestInstallPortlessInfrastructureRunsTheScriptButAddsNoProxy(t *testing.T) {
	runner := newFakeRunner("my-project")
	installer := testInstaller(t, runner)

	if err := installer.Install(context.Background(), portlessInfrastructureSpec("my-project")); err != nil {
		t.Fatalf("install: %v", err)
	}

	if !runner.contains("exec my-project") || !runner.contains("bash -s") {
		t.Errorf("want the install script to run inside my-project, got:\n%s",
			strings.Join(runner.commands(), "\n"))
	}
	if runner.contains("proxy") {
		t.Errorf("portless infrastructure must not add a proxy device:\n%s",
			strings.Join(runner.commands(), "\n"))
	}
	for _, forbidden := range []string{"launch", "init", "copy", "create"} {
		if runner.hasPrefix(forbidden + " ") {
			t.Errorf("portless infrastructure ran %q; it must reuse the project container:\n%s",
				forbidden, strings.Join(runner.commands(), "\n"))
		}
	}
}

// Stopping project-scoped infrastructure stops its unit and leaves the project
// container running.
func TestStopPortlessInfrastructureStopsOnlyTheUnit(t *testing.T) {
	runner := newFakeRunner("my-project")
	installer := testInstaller(t, runner)

	if err := installer.Stop(context.Background(), portlessInfrastructureSpec("my-project")); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if !runner.contains("systemctl stop fixture-portless") {
		t.Errorf("want the unit stopped, got:\n%s", strings.Join(runner.commands(), "\n"))
	}
	if runner.hasPrefix("stop ") {
		t.Errorf("stopping project infrastructure must not stop the project container:\n%s",
			strings.Join(runner.commands(), "\n"))
	}
}

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

func TestInstallGlobalScopeHonoursTheApplicationBase(t *testing.T) {
	runner := newFakeRunner()
	installer := testInstaller(t, runner)
	spec := serviceSpec(svc.ScopeGlobal, "futrx-app-abc123")
	spec.Application.Base = "applications:debian/12"

	if err := installer.Install(context.Background(), spec); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !runner.hasPrefix("launch applications:debian/12 futrx-app-abc123") {
		t.Errorf("want the application's own base used, got:\n%s", strings.Join(runner.commands(), "\n"))
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
	if !runner.contains("systemctl stop fixture") {
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
	if !runner.contains("systemctl disable --now fixture") {
		t.Error("want the systemd unit disabled")
	}
	if !runner.contains("rm -f /etc/systemd/system/fixture.service /etc/remote/workspace-idle.d/fixture") ||
		!runner.contains("rm -rf /etc/remote/applications/fixture") {
		t.Error("want Remote-owned service files removed")
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

func TestUninstallFailedGlobalInstallWithoutContainerIsANoop(t *testing.T) {
	runner := newFakeRunner()
	installer := testInstaller(t, runner)

	if err := installer.Uninstall(context.Background(), serviceSpec(svc.ScopeGlobal, "")); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("empty container target reached LXD: %v", runner.commands())
	}
}

// Starting an app is not installing it again. The install script provisions
// software — on Ubuntu that is an apt-get — and paying for it every time
// someone switches an app on makes a start take minutes and gives it a whole
// class of failures that have nothing to do with starting.
func TestStartStartsTheServiceWithoutReRunningTheInstallScript(t *testing.T) {
	runner := newFakeRunner("my-project")
	installer := testInstaller(t, runner)

	if err := installer.Start(context.Background(), serviceSpec(svc.ScopeProject, "my-project")); err != nil {
		t.Fatalf("start: %v", err)
	}

	if runner.contains("bash -s") {
		t.Errorf("start re-ran the install script:\n%s", strings.Join(runner.commands(), "\n"))
	}
	if !runner.contains("systemctl start fixture") {
		t.Errorf("start did not start the application's service:\n%s", strings.Join(runner.commands(), "\n"))
	}
	// The proxy is still re-added: the host port is what the user reaches, and
	// stopping released it.
	if !runner.hasPrefix("config device add my-project app-abc123 proxy") {
		t.Errorf("start did not re-add the proxy device:\n%s", strings.Join(runner.commands(), "\n"))
	}
}

// A declared healthcheck is a promise the platform keeps: the app is reported
// running only once its own probe says it is ready. The internal port is
// substituted, so an application writes the probe without knowing which port its
// instance was given.
func TestInstallRunsTheDeclaredHealthcheck(t *testing.T) {
	runner := newFakeRunner("my-project")
	installer := testInstaller(t, runner)
	spec := serviceSpec(svc.ScopeProject, "my-project")
	spec.Application.Healthcheck.Command = "fixture-ping -P {{internalPort}}"

	if err := installer.Install(context.Background(), spec); err != nil {
		t.Fatalf("install: %v", err)
	}

	if !runner.contains("fixture-ping -P 3306") {
		t.Errorf("the healthcheck did not run with the instance's port:\n%s",
			strings.Join(runner.commands(), "\n"))
	}
}

// An application that declares no probe must not be probed: "ready" for it is the
// install script returning, and inventing a check would be inventing a way for
// a healthy install to fail.
func TestInstallSkipsTheHealthcheckWhenNoneIsDeclared(t *testing.T) {
	runner := newFakeRunner("my-project")
	installer := testInstaller(t, runner)

	if err := installer.Install(context.Background(), serviceSpec(svc.ScopeProject, "my-project")); err != nil {
		t.Fatalf("install: %v", err)
	}

	if runner.contains("sh -c") {
		t.Errorf("probed an application that declares no healthcheck:\n%s",
			strings.Join(runner.commands(), "\n"))
	}
}
