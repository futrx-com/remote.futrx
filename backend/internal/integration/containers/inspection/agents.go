package inspection

import (
	"context"
	"strings"

	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// containerAgentInspector reports provider CLI and instruction readiness.
type containerAgentInspector struct {
	commands        *quickCommandRunner
	profiles        serviceprofiles.Source
	instructionHash string
}

func (i *containerAgentInspector) inspect(ctx context.Context, containerName string) []serviceproject.AgentContainerStatus {
	profiles := i.profiles.Snapshot()
	statuses := make([]serviceproject.AgentContainerStatus, 0, len(profiles))
	for _, profile := range profiles {
		status := serviceproject.AgentContainerStatus{ID: profile.ID, Label: profile.CLI.Name}
		if _, err := i.commands.run(ctx, "exec", containerName, "--", "which", profile.CLI.Binary); err == nil {
			status.Installed = true
			versionArgs := []string{"exec", containerName, "--", profile.CLI.Binary}
			versionArgs = append(versionArgs, profile.CLI.VersionArgs...)
			if len(profile.CLI.VersionArgs) > 0 {
				version, err := i.commands.run(ctx, versionArgs...)
				if err == nil {
					status.Version = strings.TrimSpace(version)
				}
			}
		}
		if profile.Instructions != nil {
			status.InstructionsPath = profile.Instructions.Path
			if _, err := i.commands.run(ctx, "exec", containerName, "--", "test", "-f", profile.Instructions.Path); err == nil {
				status.InstructionsInstalled = true
			}
			if hash, err := i.commands.run(ctx, "exec", containerName, "--", "cat", profile.Instructions.HashPath); err == nil {
				status.InstructionsInSync = strings.TrimSpace(hash) == i.instructionHash
			}
		}
		statuses = append(statuses, status)
	}
	return statuses
}
