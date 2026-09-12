// Package resources owns the shared LXD profile that carries the default
// resource envelope for every project container. Isolation in LXD is
// namespace isolation only — without cgroup limits a single workspace can
// starve the host (observed twice in 2026-07: an ffmpeg CPU peg and a node
// OOM each took the box down). The profile puts a fleet-wide ceiling on
// every container while leaving per-project overrides to the operator.
//
// The envelope itself is no longer compiled in: the service layer owns the
// policy (`internal/service/resources`, persisted to `DATA_DIR/resources.json`)
// and pushes it here through ApplyDefaults. The values below are only the
// fallback used before that first push lands.
package resources

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
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

// Defaults is the desired fleet envelope in LXD's own vocabulary. Empty
// fields are left unmanaged on the profile.
type Defaults struct {
	Memory    string
	CPU       string
	Processes string
	// Disk is the fleet root-disk quota. It is applied per container rather
	// than on the profile: a profile-level root device would have to restate
	// the storage pool, and getting that wrong breaks every container at once.
	Disk string
}

// fallbackDefaults is the envelope in force before the service layer pushes
// the operator's policy — the caps applied by hand after the 2026-07 host
// takedowns. It is deliberately generous: the host-aware derivation on first
// run replaces it within milliseconds of startup.
var fallbackDefaults = Defaults{Memory: "4GiB", CPU: "6", Processes: "2000"}

// Manager converges the managed profile definition and its attachment to
// project containers.
type Manager struct {
	runner command.Runner

	mu       sync.RWMutex
	defaults Defaults

	pool poolCache
}

// NewManager returns a Manager that issues profile operations through runner.
func NewManager(runner command.Runner) *Manager {
	return &Manager{runner: runner, defaults: fallbackDefaults}
}

// SetDefaults replaces the desired fleet envelope. The next convergence — and
// every subsequent Ensure — writes these values.
func (m *Manager) SetDefaults(defaults Defaults) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaults = defaults
}

func (m *Manager) currentDefaults() Defaults {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaults
}

// profileEntries is the desired state of the managed profile: the operator's
// envelope plus the one setting that is a capability rather than a limit.
func (m *Manager) profileEntries() [][2]string {
	defaults := m.currentDefaults()
	entries := make([][2]string, 0, 4)
	// Hard memory ceiling: the container's own OOM killer fires inside the
	// cgroup; the host never feels it.
	if defaults.Memory != "" {
		entries = append(entries, [2]string{"limits.memory", defaults.Memory})
	}
	// CPU cap below the host's core count so the host control plane (LXD,
	// sshd, backend) always has headroom even with a pegged workspace.
	if defaults.CPU != "" {
		entries = append(entries, [2]string{"limits.cpu", defaults.CPU})
	}
	// Fork-bomb guard; the kernel PID table is shared with the host.
	if defaults.Processes != "" {
		entries = append(entries, [2]string{"limits.processes", defaults.Processes})
	}
	// Chrome's own sandbox (nested user namespaces) for the Agent Browser.
	entries = append(entries, [2]string{"security.nesting", "true"})
	return entries
}

// Available reports whether the container runtime CLI is reachable.
func (m *Manager) Available() bool { return m.runner.Available() }

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
//
// The caller passes effective values (project override merged over the fleet
// default), so an empty disk here means "no quota anywhere", not "fall back".
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
		if err != nil {
			return fmt.Errorf("%s: %w; output: %s", strings.Join(args, " "), err, out)
		}
	}

	if disk == "" {
		out, err := command.RunWithTimeout(ctx, m.runner, queryTimeout, "config", "device", "unset", containerName, "root", "size")
		if err != nil && !missingDeviceOutput(out+" "+err.Error()) {
			return fmt.Errorf("config device unset %s root size: %w; output: %s", containerName, err, out)
		}
		return nil
	}

	// A `dir` pool cannot enforce a root quota. Writing one anyway either
	// fails the launch or lies to the operator; skipping it and reporting the
	// pool capability through the API is the honest option.
	if !m.quotaSupported(ctx) {
		log.Printf("resources: skipping %s disk quota %s - storage pool cannot enforce quotas", containerName, disk)
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

func missingDeviceOutput(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "doesn't exist") ||
		strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "not defined")
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
	for _, kv := range m.profileEntries() {
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
