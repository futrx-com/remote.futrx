package auth

// APIKeyService owns a single admin-supplied API-key + base-URL provider
// configuration. Unlike CodeService / DeviceService there is no host CLI and
// no OAuth handshake: the admin enters the credentials once and they are
// persisted to disk. Status never echoes the API key back.

import (
	"context"
	"errors"
	"strings"
	"sync"
)

const apiKeySubscriptionBuffer = 8

// APIKeyConfig is the admin-supplied provider configuration persisted to disk.
type APIKeyConfig struct {
	Name    string `json:"name"`
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
}

// APIKeyStatus is the streamed credential snapshot. Config is nil until a
// provider has been saved; the API key is never included.
type APIKeyStatus struct {
	Authenticated bool                `json:"authenticated"`
	Config        *APIKeyStatusConfig `json:"config,omitempty"`
}

// APIKeyStatusConfig mirrors APIKeyConfig minus the secret. It is the only
// shape ever surfaced to transports.
type APIKeyStatusConfig struct {
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	Model   string `json:"model"`
}

// APIKeyStore is the persistence contract the service depends on. The custom
// provider's file store satisfies it.
type APIKeyStore interface {
	Load() (APIKeyConfig, error)
	Save(cfg APIKeyConfig) error
}

// ErrAPIKeyMissing is returned by Save when a required field is empty.
var ErrAPIKeyMissing = errors.New("name, api key, base url, and model are required")

type APIKeyService struct {
	store APIKeyStore

	mu            sync.Mutex
	authenticated bool
	config        APIKeyStatusConfig
	subs          map[chan APIKeyStatus]struct{}
}

func NewAPIKeyService(store APIKeyStore) *APIKeyService {
	return &APIKeyService{store: store, subs: map[chan APIKeyStatus]struct{}{}}
}

func (s *APIKeyService) Authenticated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authenticated
}

func (s *APIKeyService) Status() APIKeyStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *APIKeyService) statusLocked() APIKeyStatus {
	if !s.authenticated {
		return APIKeyStatus{Authenticated: false}
	}
	cfg := s.config
	return APIKeyStatus{Authenticated: true, Config: &cfg}
}

func (s *APIKeyService) Subscribe() (<-chan APIKeyStatus, func()) {
	ch := make(chan APIKeyStatus, apiKeySubscriptionBuffer)
	s.mu.Lock()
	if s.subs == nil {
		s.subs = map[chan APIKeyStatus]struct{}{}
	}
	s.subs[ch] = struct{}{}
	status := s.statusLocked()
	s.mu.Unlock()
	ch <- status

	cancel := func() {
		s.mu.Lock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
	return ch, cancel
}

func (s *APIKeyService) Save(ctx context.Context, cfg APIKeyConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.Name == "" || cfg.APIKey == "" || cfg.BaseURL == "" || cfg.Model == "" {
		return ErrAPIKeyMissing
	}
	if s.store == nil {
		return errors.New("custom provider store is not configured")
	}
	if err := s.store.Save(cfg); err != nil {
		return err
	}
	s.mu.Lock()
	s.authenticated = true
	s.config = APIKeyStatusConfig{Name: cfg.Name, BaseURL: cfg.BaseURL, Model: cfg.Model}
	s.broadcastLocked()
	s.mu.Unlock()
	return nil
}

// Reload re-reads the persisted config at startup so a saved provider is
// reported as authenticated without an admin round-trip.
func (s *APIKeyService) Reload() {
	if s.store == nil {
		return
	}
	cfg, err := s.store.Load()
	if err != nil || strings.TrimSpace(cfg.Name) == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" {
		return
	}
	s.mu.Lock()
	s.authenticated = true
	s.config = APIKeyStatusConfig{Name: cfg.Name, BaseURL: cfg.BaseURL, Model: cfg.Model}
	s.broadcastLocked()
	s.mu.Unlock()
}

func (s *APIKeyService) broadcastLocked() {
	status := s.statusLocked()
	for ch := range s.subs {
		select {
		case ch <- status:
		default:
			delete(s.subs, ch)
			close(ch)
		}
	}
}
