package service

import (
	"context"

	servicepermission "github.com/futrx-com/remote.futrx.com/internal/rbac"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// harnessProjectResolver translates the project service's domain model into the
// narrow project view exposed to provider adapters.
type harnessProjectResolver struct {
	projects *serviceproject.Service
}

func (r harnessProjectResolver) Get(ctx context.Context, id harness.ProjectID) (harness.Project, error) {
	project, err := r.projects.Get(ctx, serviceproject.ID(id))
	return projectForHarness(project), err
}

// Start readies the container for an agent run the caller was already
// admitted to. Runs also start from background contexts that carry no
// authenticated actor, so this is a reviewed trusted internal entry point.
func (r harnessProjectResolver) Start(ctx context.Context, id harness.ProjectID) (harness.Project, error) {
	project, err := r.projects.StartForHarness(servicepermission.ContextWithSystemActor(ctx), serviceproject.ID(id))
	return projectForHarness(project), err
}

func (r harnessProjectResolver) ListSecrets(ctx context.Context, id harness.ProjectID) ([]harness.ProjectSecret, error) {
	// Execution already has access to the injected container environment. The
	// UI secrets capability controls inspection/editing, not runtime injection.
	secrets, err := r.projects.ListSecrets(servicepermission.ContextWithSystemActor(ctx), serviceproject.ID(id))
	if err != nil {
		return nil, err
	}
	if secrets == nil {
		return nil, nil
	}
	out := make([]harness.ProjectSecret, len(secrets))
	for index, secret := range secrets {
		out[index] = harness.ProjectSecret{Key: secret.Key, Value: secret.Value}
	}
	return out, nil
}

var _ harness.ProjectResolver = harnessProjectResolver{}

func projectForHarness(project serviceproject.Meta) harness.Project {
	return harness.Project{
		ID:            harness.ProjectID(project.ID),
		ContainerName: project.ContainerName,
		Status:        harness.ProjectStatus(project.Status),
	}
}
