package kimicodex

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
	return agent.ProviderKimiCodex
}

func (p *Provider) Run(ctx context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	req.Provider = agent.ProviderKimiCodex
	key, err := p.apiKey(req.AccountID)
	if err != nil {
		return err
	}
	if p.models == nil {
		return ErrKimiCodexModelDiscoveryUnavailable
	}
	models, err := p.models.Models(ctx, key)
	if err != nil {
		return err
	}
	model, err := resolveModel(models, req.Model)
	if err != nil {
		return err
	}
	req.Model = model.ID
	req.Preferences.ReasoningEffort = model.reasoningEffort(req.Preferences.ReasoningEffort)
	req.Preferences.ServiceTier = ""
	catalog, err := buildRuntimeModelCatalog(models)
	if err != nil {
		return err
	}

	cmd, err := p.buildCmd(ctx, req, p.args(req, model.ContextWindow), key, catalog, emit)
	if err != nil {
		return err
	}
	return codexharness.Run(ctx, cmd, req, configconstants.KimiCodexLabel, emit)
}

func (p *Provider) apiKey(accountIDs ...string) (string, error) {
	if p.apiKeys == nil {
		return "", ErrKimiCodexAPIKeyMissing
	}
	accountID := ""
	if len(accountIDs) > 0 {
		accountID = accountIDs[0]
	}
	if source, ok := p.apiKeys.(interface{ APIKeyFor(string) (string, bool) }); ok {
		key, found := source.APIKeyFor(accountID)
		if !found {
			return "", ErrKimiCodexAPIKeyMissing
		}
		return key, nil
	}
	key, ok := p.apiKeys.APIKey()
	if !ok {
		return "", ErrKimiCodexAPIKeyMissing
	}
	return key, nil
}
