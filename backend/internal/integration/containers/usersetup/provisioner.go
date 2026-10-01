// Package usersetup restores the project-owned development environment
// after provisioning.
//
// Convention (see agent/provisioning/assets/AGENTS.md): agents append tool
// install lines to /workspace/setup.sh. The workspace bind-mount survives
// container replacement, so the script is durable — this provisioner is the
// missing execution step that replays it inside every fresh container.
//
// Safety properties:
//   - Opt-in: absent or non-executable setup.sh means "nothing to restore".
//   - Runs as container root, matching how agents author it (apt installs).
//   - Returns errors to the caller (the launch provisioner keeps them
//     best-effort); output is truncated to a tail so apt logs can't flood.
//
// Keep the script idempotent: it re-runs on every create and mount change.
package usersetup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

const (
	setupScriptPath   = "/workspace/setup.sh"
	setupProbeTimeout = 30 * time.Second
	// setupRunTimeout budgets apt-size restores on fresh containers.
	setupRunTimeout      = 15 * time.Minute
	setupOutputTailLines = 20
)

// Provisioner replays the project setup script inside the container.
type Provisioner struct {
	runner command.Runner
}

// NewProvisioner builds a setup-script provisioner over an lxc runner.
func NewProvisioner(runner command.Runner) *Provisioner {
	return &Provisioner{runner: runner}
}

// Ensure runs /workspace/setup.sh when it exists and is executable.
// A missing script is not an error.
func (p *Provisioner) Ensure(ctx context.Context, containerName string) error {
	if !p.runner.Available() {
		return command.ErrUnavailable
	}
	if _, err := command.RunWithTimeout(
		ctx, p.runner, setupProbeTimeout,
		"exec", containerName, "--", "test", "-x", setupScriptPath,
	); err != nil {
		return nil
	}
	out, err := command.RunWithTimeout(
		ctx, p.runner, setupRunTimeout,
		"exec", containerName, "--", "bash", setupScriptPath,
	)
	if err != nil {
		return fmt.Errorf("run %s: %w: %s", setupScriptPath, err, tailLines(out))
	}
	return nil
}

func tailLines(out string) string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > setupOutputTailLines {
		lines = lines[len(lines)-setupOutputTailLines:]
	}
	return strings.Join(lines, "\n")
}
