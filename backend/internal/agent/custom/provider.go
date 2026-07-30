package custom

// The custom provider owns no host CLI: an admin supplies an API key, base
// URL, and model directly. Run wires the saved config to an OpenAI-compatible
// chat-completions endpoint, streaming the response back as assistant text
// deltas. There is no container credential sync — the API key lives on the
// host only and is read from the file store at run time.

import (
	"context"
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filecustomprovider"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

var ErrRunNotConfigured = errors.New("custom provider is not configured — save a name, api key, base url, and model first")

type ProjectResolver interface {
	Get(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error)
	Start(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error)
	ListSecrets(ctx context.Context, id serviceproject.ID) ([]serviceproject.Secret, error)
}

type Provider struct {
	projects      ProjectResolver
	containerDeps provisioning.ContainerDependencies
	profile       provisioning.Profile
	store         *filecustomprovider.Store
}

func New(projects ProjectResolver, containerDeps provisioning.ContainerDependencies, store *filecustomprovider.Store) *Provider {
	return &Provider{projects: projects, containerDeps: containerDeps, profile: Profile(), store: store}
}

func (p *Provider) ID() agent.ProviderID {
	return agent.ProviderCustom
}

func (p *Provider) Parser(req agent.RunRequest) agent.LineParser {
	return NewParser(req)
}

func (p *Provider) Run(ctx context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	if emit == nil {
		emit = func(agent.Event) {}
	}
	if req.Provider == "" {
		req.Provider = agent.ProviderCustom
	}
	if req.Fork {
		req.ResumeID = ""
	}

	cfg, err := p.loadConfig()
	if err != nil {
		return err
	}
	return runCompletion(ctx, req, cfg, emit)
}

// loadConfig reads the persisted admin config. The API key is needed at run
// time and is never surfaced by the auth service's status responses, so the
// provider reads it straight from the file store.
func (p *Provider) loadConfig() (agentauth.APIKeyConfig, error) {
	if p.store == nil {
		return agentauth.APIKeyConfig{}, ErrRunNotConfigured
	}
	cfg, err := p.store.Load()
	if err != nil {
		return agentauth.APIKeyConfig{}, err
	}
	if cfg.APIKey == "" || cfg.BaseURL == "" || cfg.Model == "" {
		return agentauth.APIKeyConfig{}, ErrRunNotConfigured
	}
	return cfg, nil
}
