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

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
)

// Provisioner installs provider-defined and embedded assets — shared agent
// instructions and skill compatibility links — in the persistent project
// workspace.
type Provisioner struct {
	runner          command.Runner
	profiles        serviceprofiles.Source
	publisher       *assets.Publisher
	instructions    []byte
	globalSkillsDir string
}

// NewProvisioner returns a workspace provisioner backed by shared container
// dependencies. Options add capabilities that are absent in deployments that
// do not configure them, such as the platform-wide global skills library.
func NewProvisioner(
	runner command.Runner,
	profileSource serviceprofiles.Source,
	publisher *assets.Publisher,
	instructions []byte,
	options ...Option,
) *Provisioner {
	provisioner := &Provisioner{
		runner:       runner,
		profiles:     profileSource,
		publisher:    publisher,
		instructions: append([]byte(nil), instructions...),
	}
	for _, option := range options {
		option(provisioner)
	}
	return provisioner
}

// EnsureAgentInstructions pushes the shared system-instructions template to
// all configured targets, grouped by hash marker. Idempotent.
func (p *Provisioner) EnsureAgentInstructions(ctx context.Context, containerName string) error {
	if !p.runner.Available() {
		return command.ErrUnavailable
	}
	targets := configuredInstructionTargets(p.profiles.Snapshot())
	if len(targets) == 0 {
		return nil
	}
	if len(p.instructions) == 0 {
		return errors.New("agent instructions not configured")
	}

	dctx, cancelD := context.WithTimeout(ctx, 30*time.Second)
	defer cancelD()
	created := map[string]bool{}
	for _, target := range targets {
		directory := path.Dir(target.Path)
		if created[directory] {
			continue
		}
		if out, err := p.runner.Run(dctx, "exec", containerName, "--",
			"install", "-d", "-m", "700", directory); err != nil {
			return fmt.Errorf("mkdir %s: %w; output: %s", directory, err, out)
		}
		created[directory] = true
	}

	type batch struct {
		hashPath string
		paths    []string
	}
	var batches []batch
	for _, target := range targets {
		index := -1
		for i := range batches {
			if batches[i].hashPath == target.HashPath {
				index = i
				break
			}
		}
		if index < 0 {
			batches = append(batches, batch{hashPath: target.HashPath})
			index = len(batches) - 1
		}
		batches[index].paths = append(batches[index].paths, target.Path)
	}
	for _, batch := range batches {
		if err := p.publisher.Push(ctx, containerName, p.instructions,
			batch.hashPath, "644", batch.paths...); err != nil {
			return err
		}
	}
	return nil
}

func configuredInstructionTargets(profiles []provisioning.Profile) []provisioning.InstructionTarget {
	targets := make([]provisioning.InstructionTarget, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Instructions != nil {
			targets = append(targets, *profile.Instructions)
		}
	}
	return targets
}
