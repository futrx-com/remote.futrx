package custom

// Auth wires the admin-supplied custom provider to the shared API-key auth
// service. The store persists {name, apiKey, baseUrl} to disk; Reload picks
// up a previously saved provider at startup so it is reported as
// authenticated without an admin round-trip.

import (
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filecustomprovider"
)

type Auth = agentauth.APIKeyService

func NewAuth(store *filecustomprovider.Store) *Auth {
	service := agentauth.NewAPIKeyService(customStoreAdapter{store: store})
	if store != nil {
		service.Reload()
	}
	return service
}

// customStoreAdapter bridges *filecustomprovider.Store to the agentauth
// APIKeyStore interface while tolerating a nil store (used when the catalog
// is enumerated without a data directory, e.g. by AgentProfiles).
type customStoreAdapter struct {
	store *filecustomprovider.Store
}

func (a customStoreAdapter) Load() (agentauth.APIKeyConfig, error) {
	if a.store == nil {
		return agentauth.APIKeyConfig{}, nil
	}
	return a.store.Load()
}

func (a customStoreAdapter) Save(cfg agentauth.APIKeyConfig) error {
	if a.store == nil {
		return nil
	}
	return a.store.Save(cfg)
}
