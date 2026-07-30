package custom

// The custom provider owns no host CLI: an admin supplies an API key and base
// URL directly. Auth is the load-bearing part; chat execution wiring is out
// of scope for this change, so Run reports that the provider is not wired to
// a CLI. The provider still satisfies the agent.Provider interface so the
// catalog registration and auth gate behave like the other providers.

import (
	"context"
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

var ErrRunNotWired = errors.New("custom provider does not run a CLI directly; chat execution is not yet wired")

type ProjectResolver interface {
	Get(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error)
	Start(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error)
	ListSecrets(ctx context.Context, id serviceproject.ID) ([]serviceproject.Secret, error)
}

type Provider struct {
	projects      ProjectResolver
	containerDeps provisioning.ContainerDependencies
	profile       provisioning.Profile
}

func New(projects ProjectResolver, containerDeps provisioning.ContainerDependencies) *Provider {
	return &Provider{projects: projects, containerDeps: containerDeps, profile: Profile()}
}

func (p *Provider) ID() agent.ProviderID {
	return agent.ProviderCustom
}

func (p *Provider) Parser(req agent.RunRequest) agent.LineParser {
	return nil
}

func (p *Provider) Run(ctx context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	if emit == nil {
		emit = func(agent.Event) {}
	}
	return ErrRunNotWired
}
