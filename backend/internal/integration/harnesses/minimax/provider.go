package minimax

import (
	"context"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/harness"
	"github.com/futrx-com/remote.futrx.com/internal/harness/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/codexharness"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/runtime"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

type Provider struct {
	projectPreparer harness.ProjectPreparer
	apiKeys         apiKeySource
	models          modelCatalogSource
	runtimeAssets   provisioning.RuntimeAssetProvisioner
	binary          string
	planUsage       tokenPlanReader
}

type apiKeySource interface {
	APIKey() (string, bool)
	APIKeyForRun(context.Context, string) (agentauth.RunAPIKey, bool, error)
	RequireRunAccess(context.Context, string) error
}

func newProvider(
	projectPreparer harness.ProjectPreparer,
	apiKeys apiKeySource,
	models modelCatalogSource,
	runtimeAssets provisioning.RuntimeAssetProvisioner,
	binary string,
	planUsage tokenPlanReader,
) *Provider {
	return &Provider{
		projectPreparer: projectPreparer,
		apiKeys:         apiKeys,
		models:          models,
		runtimeAssets:   runtimeAssets,
		binary:          binary,
		planUsage:       planUsage,
	}
}

func (p *Provider) ID() harness.ProviderID {
	return harness.ProviderMiniMax
}

func (p *Provider) Run(ctx context.Context, req harness.RunRequest, emit func(harness.Event)) error {
	req.Provider = harness.ProviderMiniMax
	if p.apiKeys == nil {
		return ErrMiniMaxAPIKeyMissing
	}
	credential, ok, err := p.apiKeys.APIKeyForRun(ctx, req.AccountID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrMiniMaxAPIKeyMissing
	}
	req.AccountID = credential.AccountID
	key := credential.Key
	if emit == nil {
		emit = func(harness.Event) {}
	}
	emit = agentruntime.EmitForAccount(emit, req.AccountID)
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
	// Preparation and model discovery may wait on external work. Recheck the
	// pinned account before starting the process with its acquired credential.
	if err := p.apiKeys.RequireRunAccess(ctx, req.AccountID); err != nil {
		return err
	}
	return codexharness.Run(ctx, cmd, req, configconstants.MiniMaxLabel, emit)
}

func (p *Provider) apiKey(accountIDs ...string) (string, error) {
	if p.apiKeys == nil {
		return "", ErrMiniMaxAPIKeyMissing
	}
	accountID := ""
	if len(accountIDs) > 0 {
		accountID = accountIDs[0]
	}
	if source, ok := p.apiKeys.(interface{ APIKeyFor(string) (string, bool) }); ok {
		key, found := source.APIKeyFor(accountID)
		if !found {
			return "", ErrMiniMaxAPIKeyMissing
		}
		return key, nil
	}
	key, ok := p.apiKeys.APIKey()
	if !ok {
		return "", ErrMiniMaxAPIKeyMissing
	}
	return key, nil
}
