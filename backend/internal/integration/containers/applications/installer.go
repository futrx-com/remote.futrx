package applications

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/applications/hosttools"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

const (
	// defaultBase is the upstream application a dedicated (global) app container is
	// launched from when the application does not specify its own base.
	defaultBase = "ubuntu:24.04"

	// containerSkillsRoot is the project source of truth for agent skills;
	// provider-specific directories are symlinks onto it.
	containerSkillsRoot = "/workspace/.agents/skills"
	serviceConfigRoot   = "/etc/remote/applications"
	serviceUnitRoot     = "/etc/systemd/system"
	workspaceIdleRoot   = "/etc/remote/workspace-idle.d"

	execTimeout    = 8 * time.Minute
	controlTimeout = 30 * time.Second
	launchTimeout  = 90 * time.Second

	// healthTimeout bounds one run of an application's readiness probe, and healthWait
	// bounds how long the probe is retried before the install is called failed.
	healthTimeout = 15 * time.Second
	healthWait    = 60 * time.Second

	// internalPortPlaceholder is what an application writes in its healthcheck
	// command to mean "the port this instance listens on inside its container".
	internalPortPlaceholder = "{{internalPort}}"
)

// Installer realizes svc.Installer against the LXD CLI. It owns everything
// lxc-facing for an app: launching the dedicated container (global scope),
// running the install script, systemd control, and the host proxy device.
type Installer struct {
	hostTools hostToolEnsurer
	runner    command.Runner
	registry  *Registry
	publisher *assets.Publisher
}

type hostToolEnsurer interface {
	Ensure(context.Context, []svc.HostTool) error
}

func NewInstaller(runner command.Runner, registry *Registry, dataDir string) *Installer {
	return &Installer{runner: runner, registry: registry, hostTools: hosttools.New(dataDir), publisher: assets.NewPublisher(runner)}
}

var _ svc.Installer = (*Installer)(nil)

// Install converges the application's container programs, optional custom
// installer, manifest service, health check, and host proxy in that order.
func (in *Installer) Install(ctx context.Context, spec svc.InstallSpec) error {
	if err := in.ensureHostTools(ctx, spec); err != nil {
		return err
	}
	if err := in.ensureContainer(ctx, spec); err != nil {
		return err
	}
	if err := in.runInstallScript(ctx, spec); err != nil {
		return err
	}
	if err := in.installService(ctx, spec); err != nil {
		return err
	}
	if err := in.awaitHealthy(ctx, spec); err != nil {
		return err
	}
	if err := in.publishSkills(ctx, spec); err != nil {
		return err
	}
	return in.ensureProxy(ctx, spec.Instance)
}

// Start brings a previously-installed app back up. It repairs everything an
// install put outside the container's own filesystem — the host tools it
// depends on, the skills it publishes into the workspace, the proxy device that
// exposes it — and starts its service.
//
// It deliberately does not re-run the install script. That script provisions
// the app: on an Ubuntu container it is an apt-get, and paying for one every
// time an app is switched on turns a start into a minutes-long operation that
// can also fail for reasons that have nothing to do with starting. Bringing an
// out-of-date instance up to its application's version is a separate decision the
// service already makes, and it routes that case to Install.
func (in *Installer) Start(ctx context.Context, spec svc.InstallSpec) error {
	if err := in.ensureHostTools(ctx, spec); err != nil {
		return err
	}
	if err := in.ensureContainer(ctx, spec); err != nil {
		return err
	}
	if err := in.publishSkills(ctx, spec); err != nil {
		return err
	}
	if err := in.startService(ctx, spec); err != nil {
		return err
	}
	if err := in.awaitHealthy(ctx, spec); err != nil {
		return err
	}
	return in.ensureProxy(ctx, spec.Instance)
}

// ensureHostTools installs what the application needs on the Remote host itself. It
// runs before anything touches the container: a guest install script that calls
// a host tool must not run until that tool is there.
func (in *Installer) ensureHostTools(ctx context.Context, spec svc.InstallSpec) error {
	if len(spec.Application.HostTools) == 0 {
		return nil
	}
	if in.hostTools == nil {
		return fmt.Errorf("host tool installer unavailable")
	}
	if err := in.hostTools.Ensure(ctx, spec.Application.HostTools); err != nil {
		return fmt.Errorf("install host dependencies: %w", err)
	}
	return nil
}

// startService starts the application's systemd unit. A global app's own container
// was just started and brings its enabled units up with it; a project app
// shares a container that stays up, so its unit is exactly what Stop stopped.
// Either way systemctl start on a running unit is a no-op.
func (in *Installer) startService(ctx context.Context, spec svc.InstallSpec) error {
	name := spec.Application.ServiceName()
	if name == "" {
		return nil
	}
	out, err := in.exec(ctx, spec.Instance.ContainerName, nil, controlTimeout, "systemctl", "start", name)
	if err != nil {
		return fmt.Errorf("start %s in %s: %w; output: %s", name, spec.Instance.ContainerName, err, tail(out))
	}
	return nil
}

// awaitHealthy runs the application's readiness probe inside the container until it
// passes. An application that declares none is ready as soon as provisioning
// and service startup return; reporting one with a probe as running before its
// own check agrees would hand the user a port that answers nothing.
func (in *Installer) awaitHealthy(ctx context.Context, spec svc.InstallSpec) error {
	command := strings.TrimSpace(spec.Application.Healthcheck.Command)
	if command == "" {
		return nil
	}
	command = strings.ReplaceAll(command, internalPortPlaceholder, strconv.Itoa(spec.Instance.InternalPort))
	deadline := time.Now().Add(healthWait)
	for {
		out, err := in.exec(ctx, spec.Instance.ContainerName, in.scriptEnv(spec), healthTimeout, "sh", "-c", command)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not become ready within %s: %w; output: %s",
				spec.Application.ID, healthWait, err, tail(out))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// Stop stops the app and releases its host port. Global apps stop their whole
// dedicated container; project apps stop just the systemd service.
func (in *Installer) Stop(ctx context.Context, spec svc.InstallSpec) error {
	inst := spec.Instance
	// Remove the proxy first so the host port is freed even if the container
	// is already gone.
	if err := in.removeDevice(ctx, inst.ContainerName, inst.DeviceName); err != nil {
		return err
	}
	if inst.Scope == svc.ScopeGlobal {
		_, _ = command.RunWithTimeout(ctx, in.runner, controlTimeout, "stop", "--force", inst.ContainerName)
		return nil
	}
	if svcName := spec.Application.ServiceName(); svcName != "" {
		_, _ = in.exec(ctx, inst.ContainerName, nil, controlTimeout, "systemctl", "stop", svcName)
	}
	return nil
}

// Uninstall removes the proxy device and, for global scope, deletes the
// dedicated container outright. For project scope it stops and disables the
// service; installed packages and data remain in the project container.
func (in *Installer) Uninstall(ctx context.Context, spec svc.InstallSpec) error {
	inst := spec.Instance
	if inst.Scope == svc.ScopeGlobal {
		// A failed legacy install may have been persisted before container target
		// resolution completed. There is no container footprint to remove in that
		// case, and passing an empty name to LXD turns Retry into a permanent error.
		if inst.ContainerName == "" {
			return nil
		}
		// Deleting the container also drops its proxy device.
		if _, err := command.RunWithTimeout(ctx, in.runner, launchTimeout, "delete", "--force", inst.ContainerName); err != nil {
			if !isMissing(err, "") {
				return fmt.Errorf("delete app container %s: %w", inst.ContainerName, err)
			}
		}
		return nil
	}
	if err := in.removeDevice(ctx, inst.ContainerName, inst.DeviceName); err != nil {
		return err
	}
	if svcName := spec.Application.ServiceName(); svcName != "" {
		_, _ = in.exec(ctx, inst.ContainerName, nil, controlTimeout, "systemctl", "disable", "--now", svcName)
		in.removeServiceFiles(ctx, spec)
	}
	in.removeSkills(ctx, spec)
	return nil
}

// publishSkills puts an application's skills where the project's agent reads them.
// A global app has no project workspace, so it publishes nothing.
func (in *Installer) publishSkills(ctx context.Context, spec svc.InstallSpec) error {
	if spec.Instance.Scope == svc.ScopeGlobal {
		return nil
	}
	for _, name := range spec.Application.Skills {
		body, ok := in.registry.Skill(spec.Application.ID, name)
		if !ok {
			return fmt.Errorf("skill %q is declared by application %q but cannot be read", name, spec.Application.ID)
		}
		dir := path.Join(containerSkillsRoot, name)
		if out, err := in.exec(ctx, spec.Instance.ContainerName, nil, controlTimeout, "install", "-d", "-m", "755", dir); err != nil {
			return fmt.Errorf("skill %q: create %s: %w; output: %s", name, dir, err, tail(out))
		}
		if err := in.publisher.Push(ctx, spec.Instance.ContainerName, body, path.Join(dir, skillHashFile), "644", path.Join(dir, skillFileName)); err != nil {
			return fmt.Errorf("skill %q: %w", name, err)
		}
	}
	return nil
}

// removeSkills takes back what publishSkills wrote. An uninstall that leaves
// the skill behind would keep telling the agent about a feature that is gone.
func (in *Installer) removeSkills(ctx context.Context, spec svc.InstallSpec) {
	if spec.Instance.Scope == svc.ScopeGlobal {
		return
	}
	for _, name := range spec.Application.Skills {
		_, _ = in.exec(ctx, spec.Instance.ContainerName, nil, controlTimeout, "rm", "-rf", path.Join(containerSkillsRoot, name))
	}
}

// Expose (re)creates only the proxy device, leaving the running service
// untouched. Used to change the external port cheaply.
func (in *Installer) Expose(ctx context.Context, spec svc.InstallSpec) error {
	return in.ensureProxy(ctx, spec.Instance)
}

// ensureContainer makes sure the target container exists and is running.
// Project containers are readied by the project service before Install is
// called, so this only launches dedicated global-app containers.
func (in *Installer) ensureContainer(ctx context.Context, spec svc.InstallSpec) error {
	if spec.Instance.Scope != svc.ScopeGlobal {
		return nil
	}
	name := spec.Instance.ContainerName
	state, err := in.containerState(ctx, name)
	if err != nil {
		return err
	}
	switch state {
	case "running":
		return nil
	case "missing":
		base := spec.Application.Base
		if base == "" {
			base = defaultBase
		}
		if out, err := command.RunWithTimeout(ctx, in.runner, launchTimeout, "launch", base, name); err != nil {
			return fmt.Errorf("launch app container %s from %s: %w; output: %s", name, base, err, out)
		}
		return in.waitNetwork(ctx, name)
	default: // stopped / frozen
		if out, err := command.RunWithTimeout(ctx, in.runner, launchTimeout, "start", name); err != nil {
			return fmt.Errorf("start app container %s: %w; output: %s", name, err, out)
		}
		return in.waitNetwork(ctx, name)
	}
}

// runInstallScript pipes the generated container build and optional custom
// installer into `bash -s` inside the container with the resolved env applied.
func (in *Installer) runInstallScript(ctx context.Context, spec svc.InstallSpec) error {
	if spec.Application.Install == "" && spec.Application.Container == nil {
		return nil
	}
	script, ok := in.registry.Script(spec.Application.ID)
	if !ok {
		return fmt.Errorf("no install script for application %q", spec.Application.ID)
	}
	out, err := in.execStdin(ctx, spec.Instance.ContainerName, in.scriptEnv(spec), execTimeout,
		strings.NewReader(string(script)), "bash", "-s")
	if err != nil {
		return fmt.Errorf("install %s: %w; output: %s", spec.Application.ID, err, tail(out))
	}
	return nil
}

// scriptEnv is the environment an application's own commands run in: its resolved
// install-time variables, plus the internal port the instance was given. The
// healthcheck gets the same one as the install script, since a probe that has
// to authenticate needs the password that script generated.
func (in *Installer) scriptEnv(spec svc.InstallSpec) map[string]string {
	env := make(map[string]string, len(spec.Instance.Env)+5)
	for k, v := range spec.Instance.Env {
		env[k] = v
	}
	// Manifest-owned values are supplied by Remote so an application-specific
	// provisioner never has to repeat them. Assign them after user inputs so an
	// env[] declaration cannot impersonate platform metadata.
	env["APP_APPLICATION_ID"] = spec.Application.ID
	env["APP_APPLICATION_NAME"] = spec.Application.Name
	env["APP_APPLICATION_VERSION"] = spec.Application.Version
	env["APP_SERVICE"] = spec.Application.ServiceName()
	env["APP_INTERNAL_PORT"] = strconv.Itoa(spec.Instance.InternalPort)
	return env
}

// installService materializes the manifest's service declaration as files
// owned by Remote, then enables and restarts the unit. Application install
// scripts never need to write systemd configuration or duplicate lifecycle
// behavior.
func (in *Installer) installService(ctx context.Context, spec svc.InstallSpec) error {
	service := spec.Application.Service
	if service == nil {
		return nil
	}

	configDir := path.Join(serviceConfigRoot, service.Name)
	if out, err := in.exec(ctx, spec.Instance.ContainerName, nil, controlTimeout,
		"install", "-d", "-m", "755", configDir, workspaceIdleRoot); err != nil {
		return fmt.Errorf("prepare service %s in %s: %w; output: %s",
			service.Name, spec.Instance.ContainerName, err, tail(out))
	}

	environment := serviceEnvironment(spec)
	unit := serviceUnit(spec, path.Join(configDir, "environment"))
	if err := in.publisher.PushVerified(ctx, spec.Instance.ContainerName, environment,
		path.Join(configDir, ".environment.sha256"), "600", path.Join(configDir, "environment")); err != nil {
		return fmt.Errorf("publish service %s environment: %w", service.Name, err)
	}
	if err := in.publisher.PushVerified(ctx, spec.Instance.ContainerName, unit,
		path.Join(configDir, ".unit.sha256"), "644", serviceUnitPath(service.Name)); err != nil {
		return fmt.Errorf("publish service %s unit: %w", service.Name, err)
	}
	if err := in.publisher.PushVerified(ctx, spec.Instance.ContainerName, []byte(service.Name+"\n"),
		path.Join(configDir, ".idle.sha256"), "644", path.Join(workspaceIdleRoot, service.Name)); err != nil {
		return fmt.Errorf("publish service %s idle declaration: %w", service.Name, err)
	}

	if out, err := in.exec(ctx, spec.Instance.ContainerName, nil, controlTimeout, "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("reload systemd for %s in %s: %w; output: %s",
			service.Name, spec.Instance.ContainerName, err, tail(out))
	}
	if out, err := in.exec(ctx, spec.Instance.ContainerName, nil, controlTimeout, "systemctl", "enable", service.Name); err != nil {
		return fmt.Errorf("enable %s in %s: %w; output: %s",
			service.Name, spec.Instance.ContainerName, err, tail(out))
	}
	if out, err := in.exec(ctx, spec.Instance.ContainerName, nil, controlTimeout, "systemctl", "restart", service.Name); err != nil {
		return fmt.Errorf("restart %s in %s: %w; output: %s",
			service.Name, spec.Instance.ContainerName, err, tail(out))
	}
	return nil
}

func serviceEnvironment(spec svc.InstallSpec) []byte {
	var contents strings.Builder
	for _, variable := range spec.Application.Service.Environment {
		contents.WriteString(variable.Key)
		contents.WriteByte('=')
		contents.WriteString(base64.StdEncoding.EncodeToString([]byte(spec.Instance.Env[variable.FromEnv])))
		contents.WriteByte('\n')
	}
	return []byte(contents.String())
}

func serviceUnit(spec svc.InstallSpec, environmentPath string) []byte {
	service := spec.Application.Service
	description := service.Description
	if description == "" {
		description = spec.Application.Name
	}
	command := make([]string, len(service.Command))
	for index, argument := range service.Command {
		argument = strings.ReplaceAll(argument, internalPortPlaceholder, strconv.Itoa(spec.Instance.InternalPort))
		command[index] = systemdArgument(argument)
	}

	var unit strings.Builder
	fmt.Fprintf(&unit, "[Unit]\nDescription=%s\nAfter=network.target\n\n", systemdValue(description))
	unit.WriteString("[Service]\nType=simple\n")
	fmt.Fprintf(&unit, "EnvironmentFile=%s\nExecStart=%s\n", environmentPath, strings.Join(command, " "))
	if service.Restart != "" {
		fmt.Fprintf(&unit, "Restart=%s\n", service.Restart)
	}
	if service.RestartSec > 0 {
		fmt.Fprintf(&unit, "RestartSec=%ds\n", service.RestartSec)
	}
	if service.User != "" {
		fmt.Fprintf(&unit, "User=%s\n", service.User)
	}
	if service.Group != "" {
		fmt.Fprintf(&unit, "Group=%s\n", service.Group)
	}
	if service.Hardening.NoNewPrivileges {
		unit.WriteString("NoNewPrivileges=true\n")
	}
	if service.Hardening.PrivateTmp {
		unit.WriteString("PrivateTmp=true\n")
	}
	if service.Hardening.ProtectHome {
		unit.WriteString("ProtectHome=true\n")
	}
	if service.Hardening.ProtectSystem != "" {
		fmt.Fprintf(&unit, "ProtectSystem=%s\n", service.Hardening.ProtectSystem)
	}
	unit.WriteString("\n[Install]\nWantedBy=multi-user.target\n")
	return []byte(unit.String())
}

func systemdArgument(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + systemdValue(value) + `"`
}

func systemdValue(value string) string {
	value = strings.ReplaceAll(value, "%", "%%")
	return strings.ReplaceAll(value, "$", "$$")
}

func serviceUnitPath(name string) string {
	return path.Join(serviceUnitRoot, name+".service")
}

func (in *Installer) removeServiceFiles(ctx context.Context, spec svc.InstallSpec) {
	service := spec.Application.Service
	if service == nil {
		return
	}
	configDir := path.Join(serviceConfigRoot, service.Name)
	_, _ = in.exec(ctx, spec.Instance.ContainerName, nil, controlTimeout, "rm", "-f",
		serviceUnitPath(service.Name), path.Join(workspaceIdleRoot, service.Name))
	_, _ = in.exec(ctx, spec.Instance.ContainerName, nil, controlTimeout, "rm", "-rf", configDir)
	_, _ = in.exec(ctx, spec.Instance.ContainerName, nil, controlTimeout, "systemctl", "daemon-reload")
}

// ensureProxy (re)creates the host proxy device for an instance. It removes any
// existing device of the same name first so a changed port takes effect.
func (in *Installer) ensureProxy(ctx context.Context, inst svc.Instance) error {
	// Portless infrastructure has no device name and therefore no proxy to create.
	if inst.DeviceName == "" {
		return nil
	}
	if err := in.removeDevice(ctx, inst.ContainerName, inst.DeviceName); err != nil {
		return err
	}
	proto := string(inst.Protocol)
	if proto == "" {
		proto = "tcp"
	}
	bind := inst.BindAddress
	if bind == "" {
		bind = "127.0.0.1"
	}
	listen := fmt.Sprintf("%s:%s:%d", proto, bind, inst.ExternalPort)
	connect := fmt.Sprintf("%s:127.0.0.1:%d", proto, inst.InternalPort)
	args := []string{
		"config", "device", "add", inst.ContainerName, inst.DeviceName, "proxy",
		"listen=" + listen,
		"connect=" + connect,
		"bind=host",
	}
	if out, err := command.RunWithTimeout(ctx, in.runner, controlTimeout, args...); err != nil {
		return fmt.Errorf("add proxy device %s (%s->%d): %w; output: %s",
			inst.DeviceName, listen, inst.InternalPort, err, out)
	}
	return nil
}

func (in *Installer) removeDevice(ctx context.Context, container, device string) error {
	if container == "" || device == "" {
		return nil
	}
	out, err := command.RunWithTimeout(ctx, in.runner, controlTimeout, "config", "device", "remove", container, device)
	if err != nil {
		if isMissing(err, out) {
			return nil
		}
		return fmt.Errorf("remove device %s: %w; output: %s", device, err, out)
	}
	return nil
}

// containerState returns "running", "stopped", "frozen", or "missing".
func (in *Installer) containerState(ctx context.Context, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("container name is required")
	}
	out, err := command.RunWithTimeout(ctx, in.runner, controlTimeout, "info", name)
	if err != nil {
		if isMissing(err, out) {
			return "missing", nil
		}
		return "", fmt.Errorf("lxc info %s: %w; output: %s", name, err, out)
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.ToLower(line))
		if strings.HasPrefix(line, "status:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "status:")), nil
		}
	}
	return "", fmt.Errorf("could not parse state of %s from: %s", name, tail(out))
}

// waitNetwork blocks until the container has an IPv4 address, so the first
// apt-get inside the install script can reach the network.
func (in *Installer) waitNetwork(ctx context.Context, name string) error {
	deadline := time.Now().Add(60 * time.Second)
	for {
		out, _ := command.RunWithTimeout(ctx, in.runner, controlTimeout,
			"exec", name, "--", "sh", "-c", "ip -4 route get 1.1.1.1 >/dev/null 2>&1 && echo ok")
		if strings.Contains(out, "ok") {
			return nil
		}
		if time.Now().After(deadline) {
			return nil // best-effort; let the install script surface any real failure
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (in *Installer) exec(ctx context.Context, container string, env map[string]string, timeout time.Duration, cmd ...string) (string, error) {
	return in.execStdin(ctx, container, env, timeout, nil, cmd...)
}

func (in *Installer) execStdin(ctx context.Context, container string, env map[string]string, timeout time.Duration, stdin io.Reader, cmd ...string) (string, error) {
	args := []string{"exec", container}
	for _, k := range sortedKeys(env) {
		args = append(args, "--env", k+"="+env[k])
	}
	args = append(args, "--")
	args = append(args, cmd...)
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if stdin != nil {
		return in.runner.RunStdin(tctx, stdin, args...)
	}
	return in.runner.Run(tctx, args...)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isMissing(err error, out string) bool {
	if err == nil {
		return false
	}
	l := strings.ToLower(out)
	return strings.Contains(l, "not found") || strings.Contains(l, "no such") ||
		strings.Contains(l, "doesn't exist") || strings.Contains(l, "does not exist")
}

func tail(s string) string {
	const max = 2000
	if len(s) <= max {
		return s
	}
	return "..." + s[len(s)-max:]
}
