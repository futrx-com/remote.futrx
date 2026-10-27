package service

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// agentProjectResolver translates the project service's domain model into the
// narrow project view exposed to provider adapters.
type agentProjectResolver struct {
	projects *serviceproject.Service
}

func (r agentProjectResolver) Get(ctx context.Context, id agent.ProjectID) (agent.Project, error) {
	project, err := r.projects.Get(ctx, serviceproject.ID(id))
	return projectForAgent(project), err
}

func (r agentProjectResolver) Start(ctx context.Context, id agent.ProjectID) (agent.Project, error) {
	project, err := r.projects.Start(ctx, serviceproject.ID(id))
	return projectForAgent(project), err
}

func (r agentProjectResolver) ListSecrets(ctx context.Context, id agent.ProjectID) ([]agent.ProjectSecret, error) {
	secrets, err := r.projects.ListSecrets(ctx, serviceproject.ID(id))
	if err != nil {
		return nil, err
	}
	if secrets == nil {
		return nil, nil
	}
	out := make([]agent.ProjectSecret, len(secrets))
	for index, secret := range secrets {
		out[index] = agent.ProjectSecret{Key: secret.Key, Value: secret.Value}
	}
	return out, nil
}

var _ agent.ProjectResolver = agentProjectResolver{}

func projectForAgent(project serviceproject.Meta) agent.Project {
	return agent.Project{
		ID:            agent.ProjectID(project.ID),
		ContainerName: project.ContainerName,
		Status:        agent.ProjectStatus(project.Status),
	}
}
