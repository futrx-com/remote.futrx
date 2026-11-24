// Package workspace provisions shared agent instructions and workspace skill
// topology inside project containers.
package workspace

// The shared instructions reach the agent as system instructions at launch.
// Earlier releases wrote them into each provider home instead, where the CLIs
// loaded them as user memory; those generated copies are removed here so the
// text is not loaded twice.

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

const retireTimeout = 30 * time.Second

// Provisioner installs provider-defined and embedded assets — skill
// compatibility links — in the persistent project workspace, and supplies the
// shared agent instructions.
type Provisioner struct {
	runner       command.Runner
	profiles     serviceprofiles.Source
	publisher    *assets.Publisher
	instructions string
	// retired records containers already confirmed free of legacy copies.
	// Nothing writes them any more, so later runs skip the checks.
	retired sync.Map
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
		instructions: string(instructions),
	}
}

// HarnessInstructions returns the rendered shared instructions that providers
// pass to the agent as system instructions.
func (p *Provisioner) HarnessInstructions() string {
	return p.instructions
}

// retireScript removes one legacy file in a single shell. The file is first
// renamed to a staging name, which is atomic, and only the staged copy is
// checked, so a file replaced at the same moment is never the one deleted.
// A staged file that does not match goes back to its path, or stays under the
// staging name if the path was taken meanwhile. It prints absent, removed or
// kept; anything else, including a failed exec, means unknown.
const retireScript = `path=$1; marker=$2; stage="$path.remote-retire.$$"
if [ ! -e "$path" ] && [ ! -L "$path" ]; then echo absent; exit 0; fi
if [ -L "$path" ] || [ ! -f "$path" ]; then echo kept; exit 0; fi
want=$(cat -- "$marker" 2>/dev/null) || want=
if [ -z "$want" ]; then echo kept; exit 0; fi
mv -- "$path" "$stage" || { echo kept; exit 0; }
if [ ! -L "$stage" ] && [ -f "$stage" ] && [ "$(sha256sum < "$stage" | cut -d' ' -f1)" = "$want" ]; then
  rm -f -- "$stage" && { echo removed; exit 0; }
fi
if [ ! -e "$path" ] && [ ! -L "$path" ]; then mv -- "$stage" "$path"; fi
echo kept`

// RetireHarnessInstructions removes instruction files an earlier release
// generated in provider homes. A file is removed only when it is a regular
// file whose bytes still hash to the marker written next to it, so a copy
// someone edited or replaced is kept. A marker is removed only once every file
// that shares it is confirmed gone. Idempotent.
func (p *Provisioner) RetireHarnessInstructions(ctx context.Context, containerName string) error {
	if !p.runner.Available() {
		return command.ErrUnavailable
	}
	if _, done := p.retired.Load(containerName); done {
		return nil
	}
	targets := legacyInstructionTargets(p.profiles.Snapshot())
	if len(targets) == 0 {
		return nil
	}
	rctx, cancel := context.WithTimeout(ctx, retireTimeout)
	defer cancel()

	markerOrder := []string{}
	markerClear := map[string]bool{}
	for _, target := range targets {
		if _, seen := markerClear[target.HashPath]; !seen {
			markerOrder = append(markerOrder, target.HashPath)
			markerClear[target.HashPath] = true
		}
		out, err := p.runner.Run(rctx, "exec", containerName, "--",
			"sh", "-c", retireScript, "sh", target.Path, target.HashPath)
		switch strings.TrimSpace(out) {
		case "absent", "removed":
			if err == nil {
				continue
			}
		}
		markerClear[target.HashPath] = false
	}
	clean := true
	for _, hashPath := range markerOrder {
		if !markerClear[hashPath] {
			clean = false
			continue
		}
		if _, err := p.runner.Run(rctx, "exec", containerName, "--", "rm", "-f", "--", hashPath); err != nil {
			clean = false
		}
	}
	if clean {
		p.retired.Store(containerName, true)
	}
	return nil
}

func legacyInstructionTargets(profiles []provisioning.Profile) []provisioning.InstructionTarget {
	targets := make([]provisioning.InstructionTarget, 0, len(profiles))
	for _, profile := range profiles {
		if profile.LegacyInstructions != nil {
			targets = append(targets, *profile.LegacyInstructions)
		}
	}
	return targets
}
