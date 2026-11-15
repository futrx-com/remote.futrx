package applications

import (
	"context"
	"fmt"
)

// uninstalls all applications belong to a project.
func (s *Service) DeleteProjectApplications(ctx context.Context, projectID string) error {
	instances, err := s.store.ListProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list project applications: %w", err)
	}

	for _, inst := range instances {
		if inst.Scope != ScopeProject || inst.ProjectID != projectID {
			continue
		}

		install := PackageInstall{
			InstanceID: inst.ID,
		}

		if err := s.uninstallForPackageRemoval(ctx, inst.ApplicationID, install); err != nil {
			return fmt.Errorf(
				"uninstall application %s (instance %s): %w",
				inst.ApplicationID,
				inst.ID,
				err,
			)
		}
	}

	return nil
}
