package inspection

import (
	"context"
	"strings"

	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// containerAgentInspector reports provider CLI readiness. Shared instructions
// are passed at launch, so there is no instruction file to probe.
type containerAgentInspector struct {
	commands *quickCommandRunner
	profiles serviceprofiles.Source
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
		statuses = append(statuses, status)
	}
	return statuses
}
