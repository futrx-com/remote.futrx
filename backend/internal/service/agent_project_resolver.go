package service

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
	servicepermission "github.com/futrx-com/remote.futrx.com/internal/rbac"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// agentProjectResolver translates the project service's domain model into the
// narrow project view exposed to provider adapters.
type agentProjectResolver struct {
	projects *serviceproject.Service
}

func (r agentProjectResolver) Get(ctx context.Context, id harness.ProjectID) (harness.Project, error) {
	project, err := r.projects.Get(ctx, serviceproject.ID(id))
	return projectForAgent(project), err
}

// Start readies the container for an agent run the caller was already
// admitted to. Runs also start from background contexts that carry no
// authenticated actor, so this is a reviewed trusted internal entry point.
func (r agentProjectResolver) Start(ctx context.Context, id harness.ProjectID) (harness.Project, error) {
	project, err := r.projects.StartForAgent(servicepermission.ContextWithSystemActor(ctx), serviceproject.ID(id))
	return projectForAgent(project), err
}

func (r agentProjectResolver) ListSecrets(ctx context.Context, id harness.ProjectID) ([]harness.ProjectSecret, error) {
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

var _ harness.ProjectResolver = agentProjectResolver{}

func projectForAgent(project serviceproject.Meta) harness.Project {
	return harness.Project{
		ID:            harness.ProjectID(project.ID),
		ContainerName: project.ContainerName,
		Status:        harness.ProjectStatus(project.Status),
	}
}
