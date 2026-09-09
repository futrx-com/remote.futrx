// Package resources owns the shared LXD profile that carries the default
// resource envelope for every project container. Isolation in LXD is
// namespace isolation only — without cgroup limits a single workspace can
// starve the host (observed twice in 2026-07: an ffmpeg CPU peg and a node
// OOM each took the box down). The profile puts a fleet-wide ceiling on
// every container while leaving per-project overrides to the operator.
package resources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

const (
	// ProfileName is the backend-managed LXD profile attached to every
	// project container alongside `default`. LXD precedence: container-local
	// config wins over profile config, so a per-project
	// `lxc config set <container> limits.memory 8GiB` overrides the fleet
	// default without touching the profile.
	ProfileName = "futrx-workspace"

	queryTimeout = 10 * time.Second
)

// profileConfig is the desired state of the managed profile. The limits
// mirror the caps the operator applied by hand after the 2026-07 host
// takedowns. The backend converges the profile to these values on every
// Launch — edit HERE and redeploy to change the fleet default; hand-edits
// via `lxc profile edit` are reverted on the next convergence.
var profileConfig = [][2]string{
	// Hard memory ceiling: the container's own OOM killer fires inside the
	// cgroup; the host never feels it.
	{"limits.memory", "8GiB"},
	// CPU cap below the host's core count so the host control plane (LXD,
	// sshd, backend) always has headroom even with a pegged workspace.
	{"limits.cpu", "2"},
	// Fork-bomb guard; the kernel PID table is shared with the host.
	{"limits.processes", "2000"},
	// Chrome's own sandbox (nested user namespaces) for the Agent Browser.
	{"security.nesting", "true"},
}

// Manager converges the managed profile definition and its attachment to
// project containers.
type Manager struct {
	runner          command.Runner
	viteAllowedHost string
}

// NewManager returns a Manager that issues profile operations through runner.
func NewManager(runner command.Runner, viteAllowedHost ...string) *Manager {
	host := ""
	if len(viteAllowedHost) > 0 {
		host = strings.TrimSpace(viteAllowedHost[0])
	}
	return &Manager{runner: runner, viteAllowedHost: host}
}

func (m *Manager) desiredProfileConfig() [][2]string {
	config := append([][2]string(nil), profileConfig...)
	if m.viteAllowedHost != "" {
		config = append(config, [2]string{
			"environment.__VITE_ADDITIONAL_SERVER_ALLOWED_HOSTS",
			m.viteAllowedHost,
		})
	}
	return config
}

// Ensure converges the profile definition, then attaches the profile to the
// container. Idempotent and cheap on the healthy path (a handful of local
// reads). Called on every Launch — including for pre-existing containers —
// so old workspaces converge to the resource envelope without recreation.
//
// Attaching to a RUNNING container applies the limits live. If the container
// currently uses more memory than the cap, the kernel reclaims down to it
// (worst case the container-internal OOM killer trims the offender) — the
// intended behavior for a workspace that would otherwise threaten the host.
func (m *Manager) Ensure(ctx context.Context, containerName string) error {
	if err := m.ensureProfile(ctx); err != nil {
		return err
	}
	return m.ensureAttached(ctx, containerName)
}

// SetLimits writes container-local overrides, which take precedence over the
// managed profile. Empty values remove the corresponding override. CPU and
// memory are instance config keys; root-disk quota is a disk-device property.
func (m *Manager) SetLimits(ctx context.Context, containerName, cpu, memory, disk string) error {
	for _, limit := range []struct {
		key   string
		value string
	}{
		{key: "limits.cpu", value: cpu},
		{key: "limits.memory", value: memory},
	} {
		args := []string{"config", "set", containerName, limit.key, limit.value}
		if limit.value == "" {
			args = []string{"config", "unset", containerName, limit.key}
		}
		out, err := command.RunWithTimeout(ctx, m.runner, queryTimeout, args...)
		if err != nil && !(limit.value == "" && missingConfigOutput(out+" "+err.Error())) {
			return fmt.Errorf("%s: %w; output: %s", strings.Join(args, " "), err, out)
		}
	}

	if disk == "" {
		out, err := command.RunWithTimeout(ctx, m.runner, queryTimeout, "config", "device", "unset", containerName, "root", "size")
		if err != nil && !missingDeviceOutput(out+" "+err.Error()) && !inheritedDeviceOutput(out+" "+err.Error()) {
			return fmt.Errorf("config device unset %s root size: %w; output: %s", containerName, err, out)
		}
		return nil
	}

	out, err := command.RunWithTimeout(ctx, m.runner, queryTimeout, "config", "device", "override", containerName, "root", "size="+disk)
	if err == nil {
		return nil
	}
	if !strings.Contains(strings.ToLower(out+" "+err.Error()), "already exists") {
		return fmt.Errorf("config device override %s root: %w; output: %s", containerName, err, out)
	}
	out, err = command.RunWithTimeout(ctx, m.runner, queryTimeout, "config", "device", "set", containerName, "root", "size", disk)
	if err != nil {
		return fmt.Errorf("config device set %s root size: %w; output: %s", containerName, err, out)
	}
	return nil
}

func missingConfigOutput(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "not currently set") ||
		strings.Contains(lower, "is not set")
}

func missingDeviceOutput(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "doesn't exist") ||
		strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "not defined")
}

func inheritedDeviceOutput(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "cannot be modified for individual instance") ||
		strings.Contains(lower, "override device")
}

func (m *Manager) ensureProfile(ctx context.Context) error {
	if !m.runner.Available() {
		return command.ErrUnavailable
	}
	_, showErr := command.RunWithTimeout(ctx, m.runner, queryTimeout, "profile", "show", ProfileName)
	if showErr != nil {
		out, err := command.RunWithTimeout(ctx, m.runner, queryTimeout, "profile", "create", ProfileName)
		// A concurrent Launch may have created it between show and create.
		if err != nil && !strings.Contains(out, "already exists") {
			return fmt.Errorf("profile create %s: %w; output: %s", ProfileName, err, out)
		}
	}
	for _, kv := range m.desiredProfileConfig() {
		key, want := kv[0], kv[1]
		current, _ := command.RunWithTimeout(ctx, m.runner, queryTimeout, "profile", "get", ProfileName, key)
		if strings.TrimSpace(current) == want {
			continue
		}
		out, err := command.RunWithTimeout(ctx, m.runner, queryTimeout, "profile", "set", ProfileName, key, want)
		if err != nil {
			return fmt.Errorf("profile set %s %s: %w; output: %s", ProfileName, key, err, out)
		}
	}
	return nil
}

func (m *Manager) ensureAttached(ctx context.Context, containerName string) error {
	shown, err := command.RunWithTimeout(ctx, m.runner, queryTimeout, "config", "show", containerName)
	if err != nil {
		return fmt.Errorf("config show %s: %w; output: %s", containerName, err, shown)
	}
	// `lxc config show` lists attached profiles as YAML entries ("- default").
	if strings.Contains(shown, "- "+ProfileName) {
		return nil
	}
	out, err := command.RunWithTimeout(ctx, m.runner, queryTimeout, "profile", "add", containerName, ProfileName)
	if err != nil {
		return fmt.Errorf("profile add %s %s: %w; output: %s", containerName, ProfileName, err, out)
	}
	return nil
}
