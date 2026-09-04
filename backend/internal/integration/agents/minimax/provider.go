package minimax

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/integration/agents/codexharness"
)

type Provider struct {
	projectPreparer agent.ProjectPreparer
	apiKeys         apiKeySource
	models          modelCatalogSource
	runtimeAssets   provisioning.RuntimeAssetProvisioner
	binary          string
}

type apiKeySource interface {
	APIKey() (string, bool)
}

func newProvider(
	projectPreparer agent.ProjectPreparer,
	apiKeys apiKeySource,
	models modelCatalogSource,
	runtimeAssets provisioning.RuntimeAssetProvisioner,
	binary string,
) *Provider {
	return &Provider{
		projectPreparer: projectPreparer,
		apiKeys:         apiKeys,
		models:          models,
		runtimeAssets:   runtimeAssets,
		binary:          binary,
	}
}

func (p *Provider) ID() agent.ProviderID {
	return agent.ProviderMiniMax
}

func (p *Provider) Run(ctx context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	req.Provider = agent.ProviderMiniMax
	key, err := p.apiKey()
	if err != nil {
		return err
	}
	if p.models == nil {
		return ErrMiniMaxModelDiscoveryUnavailable
	}
	models, err := p.models.Models(ctx, key)
	if err != nil {
		return err
	}
	req.Model, err = resolveMiniMaxModel(models, req.Model)
	if err != nil {
		return err
	}
	req.Preferences.ReasoningEffort = miniMaxReasoningEffort(req.Model, req.Preferences.ReasoningEffort)
	req.Preferences.ServiceTier = ""
	catalog, err := buildRuntimeModelCatalog(models)
	if err != nil {
		return err
	}

	cmd, err := p.buildCmd(ctx, req, p.args(req), key, catalog, emit)
	if err != nil {
		return err
	}
	return codexharness.Run(ctx, cmd, req, configconstants.MiniMaxLabel, emit)
}

func (p *Provider) apiKey() (string, error) {
	if p.apiKeys == nil {
		return "", ErrMiniMaxAPIKeyMissing
	}
	key, ok := p.apiKeys.APIKey()
	if !ok {
		return "", ErrMiniMaxAPIKeyMissing
	}
	return key, nil
}
