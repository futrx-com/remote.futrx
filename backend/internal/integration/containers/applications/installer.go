package applications

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

const (
	// defaultBase is the upstream image a dedicated (global) app container is
	// launched from when the image does not specify its own base.
	defaultBase = "ubuntu:24.04"

	execTimeout    = 8 * time.Minute
	controlTimeout = 30 * time.Second
	launchTimeout  = 90 * time.Second
)

// Installer realizes svc.Installer against the LXD CLI. It owns everything
// lxc-facing for an app: launching the dedicated container (global scope),
// running the install script, systemd control, and the host proxy device.
type Installer struct {
	runner   command.Runner
	registry *Registry
}

// NewInstaller builds an installer over an lxc command runner and the catalog
// registry (used to fetch install scripts).
func NewInstaller(runner command.Runner, registry *Registry) *Installer {
	return &Installer{runner: runner, registry: registry}
}

var _ svc.Installer = (*Installer)(nil)

// Install (re)runs the image's install script inside the target container and
// (re)creates the proxy device that exposes it on the host.
func (in *Installer) Install(ctx context.Context, spec svc.InstallSpec) error {
	if err := in.ensureContainer(ctx, spec); err != nil {
		return err
	}
	if err := in.runInstallScript(ctx, spec); err != nil {
		return err
	}
	return in.ensureProxy(ctx, spec.Instance)
}

// Start brings a previously-installed app back up: it re-runs the (idempotent)
// install script so the current port takes effect, and re-adds the proxy.
func (in *Installer) Start(ctx context.Context, spec svc.InstallSpec) error {
	return in.Install(ctx, spec)
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
	if svcName := spec.Image.Service; svcName != "" {
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
	if svcName := spec.Image.Service; svcName != "" {
		_, _ = in.exec(ctx, inst.ContainerName, nil, controlTimeout, "systemctl", "disable", "--now", svcName)
	}
	return nil
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
		base := spec.Image.Base
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

// runInstallScript pipes the image's install script into `bash -s` inside the
// container with the resolved env applied.
func (in *Installer) runInstallScript(ctx context.Context, spec svc.InstallSpec) error {
	script, ok := in.registry.Script(spec.Image.ID)
	if !ok {
		return fmt.Errorf("no install script for image %q", spec.Image.ID)
	}
	env := map[string]string{"APP_INTERNAL_PORT": strconv.Itoa(spec.Instance.InternalPort)}
	for k, v := range spec.Instance.Env {
		env[k] = v
	}
	out, err := in.execStdin(ctx, spec.Instance.ContainerName, env, execTimeout,
		strings.NewReader(string(script)), "bash", "-s")
	if err != nil {
		return fmt.Errorf("install %s: %w; output: %s", spec.Image.ID, err, tail(out))
	}
	return nil
}

// ensureProxy (re)creates the host proxy device for an instance. It removes any
// existing device of the same name first so a changed port takes effect.
func (in *Installer) ensureProxy(ctx context.Context, inst svc.Instance) error {
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
	return "", fmt.Errorf("could not parse state of %s from: %s", name, out)
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
