// Package workspace provisions shared agent instructions and workspace skill
// topology inside project containers.
package workspace

// Agent-instruction provisioning ships the shared template to every target
// declared by the configured profiles.

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

// Provisioner installs provider-defined and embedded assets — shared agent
// instructions and skill compatibility links — in the persistent project
// workspace.
type Provisioner struct {
	runner       command.Runner
	profiles     serviceprofiles.Source
	publisher    *assets.Publisher
	instructions []byte
}

// NewProvisioner returns a workspace provisioner backed by shared container
// dependencies.
func NewProvisioner(
	runner command.Runner,
	profileSource serviceprofiles.Source,
	publisher *assets.Publisher,
	instructions []byte,
) *Provisioner {
	return &Provisioner{
		runner:       runner,
		profiles:     profileSource,
		publisher:    publisher,
		instructions: append([]byte(nil), instructions...),
	}
}

// EnsureHarnessInstructions publishes native system-prompt assets and returns
// the trusted template for providers that accept instruction text directly.
func (p *Provisioner) EnsureHarnessInstructions(ctx context.Context, containerName string) (string, error) {
	if !p.runner.Available() {
		return "", command.ErrUnavailable
	}
	targets := configuredInstructionTargets(p.profiles.Snapshot())
	if len(targets) == 0 {
		return string(p.instructions), nil
	}
	if len(p.instructions) == 0 {
		return "", errors.New("agent instructions not configured")
	}

	for _, target := range targets {
		dctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		out, err := p.runner.Run(dctx, "exec", containerName, "--",
			"install", "-d", "-m", "700", path.Dir(target.Path))
		cancel()
		if err != nil {
			return "", fmt.Errorf("mkdir %s: %w; output: %s", path.Dir(target.Path), err, out)
		}
		// Provider homes are writable inside the container. Verify the bytes,
		// not just a sidecar marker, before using these as privileged guidance.
		if err := p.publisher.PushVerified(ctx, containerName, target.Render(p.instructions),
			target.HashPath, "644", target.Path); err != nil {
			return "", err
		}
		if target.LegacyPath != "" && target.LegacyHashPath != "" {
			if _, err := command.RunWithTimeout(ctx, p.runner, 30*time.Second,
				"exec", containerName, "--", "sh", "-eu", "-c", removeManagedInstructions,
				"remote-instructions-migration", target.LegacyPath, target.LegacyHashPath); err != nil {
				return "", fmt.Errorf("migrate %s: %w", target.LegacyPath, err)
			}
		}
	}
	return string(p.instructions), nil
}

// Keep the old marker: Claude and Codex shared it, so deleting it while migrating
// one profile would prevent recognizing the other profile's managed file.
const removeManagedInstructions = `if [ -f "$1" ] && [ -f "$2" ]; then
    expected=$(cat "$2")
    actual=$(sha256sum "$1")
    if [ "${actual%% *}" = "$expected" ]; then
        rm -- "$1"
    fi
fi`

func configuredInstructionTargets(profiles []provisioning.Profile) []provisioning.InstructionTarget {
	targets := make([]provisioning.InstructionTarget, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Instructions != nil {
			targets = append(targets, *profile.Instructions)
		}
	}
	return targets
}
