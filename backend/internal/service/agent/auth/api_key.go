package auth

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

var (
	ErrAPIKeyRequired         = errors.New("API key is required")
	ErrAPIKeyRejected         = errors.New("API key is invalid or unauthorized")
	ErrAPIKeyStoreUnavailable = errors.New("API key store is unavailable")
)

// APIKeyStore persists provider API keys without exposing them through auth
// snapshots or capability responses. An empty key means the provider has not
// been configured.
type APIKeyStore interface {
	AgentAPIKey(context.Context, agent.ProviderID) (string, error)
	SaveAgentAPIKey(context.Context, agent.ProviderID, string) error
	DeleteAgentAPIKey(context.Context, agent.ProviderID) error
}

// APIKeyValidator verifies a credential with its provider before the service
// persists it or publishes an authenticated status.
type APIKeyValidator interface {
	ValidateAPIKey(context.Context, string) error
}

// APIKeyFormatValidator lets a provider reject a previously stored credential
// locally when its supported credential class changes. It must not perform
// network I/O; remote validation remains part of ValidateAPIKey on mutation.
type APIKeyFormatValidator interface {
	ValidateAPIKeyFormat(string) error
}

type APIKeyValidatorFunc func(context.Context, string) error

func (f APIKeyValidatorFunc) ValidateAPIKey(ctx context.Context, key string) error {
	return f(ctx, key)
}

// APIKeyStatus is the only public state for a managed API key. The credential
// itself is write-only at the transport boundary.
type APIKeyStatus struct {
	Authenticated bool `json:"authenticated"`
}

// APIKeyService owns one provider's stored credential and broadcasts only
// configured/unconfigured transitions to auth subscribers.
type APIKeyService struct {
	id        agent.ProviderID
	store     APIKeyStore
	validator APIKeyValidator

	mutationMu sync.Mutex
	mu         sync.RWMutex
	key        string
	subs       map[chan APIKeyStatus]struct{}
}

func NewAPIKeyService(
	ctx context.Context,
	id agent.ProviderID,
	store APIKeyStore,
	validator APIKeyValidator,
) (*APIKeyService, error) {
	service := &APIKeyService{
		id: id, store: store, validator: validator,
		subs: make(map[chan APIKeyStatus]struct{}),
	}
	if store == nil {
		return service, nil
	}
	key, err := store.AgentAPIKey(ctx, id)
	if err != nil {
		return nil, err
	}
	service.key = strings.TrimSpace(key)
	if formatValidator, ok := validator.(APIKeyFormatValidator); ok {
		if err := formatValidator.ValidateAPIKeyFormat(service.key); err != nil {
			service.key = ""
		}
	}
	return service, nil
}

func (s *APIKeyService) Authenticated() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.key != ""
}

func (s *APIKeyService) APIKey() (string, bool) {
	if s == nil {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.key, s.key != ""
}

func (s *APIKeyService) Status() APIKeyStatus {
	return APIKeyStatus{Authenticated: s.Authenticated()}
}

func (s *APIKeyService) Subscribe() (<-chan APIKeyStatus, func()) {
	ch := make(chan APIKeyStatus, subscriptionBuffer)
	s.mu.Lock()
	if s.subs == nil {
		s.subs = make(map[chan APIKeyStatus]struct{})
	}
	s.subs[ch] = struct{}{}
	status := APIKeyStatus{Authenticated: s.key != ""}
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

func (s *APIKeyService) Set(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return ErrAPIKeyRequired
	}
	if s == nil || s.store == nil {
		return ErrAPIKeyStoreUnavailable
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if s.validator != nil {
		if err := s.validator.ValidateAPIKey(ctx, key); err != nil {
			return err
		}
	}
	if err := s.store.SaveAgentAPIKey(ctx, s.id, key); err != nil {
		return err
	}
	s.mu.Lock()
	s.key = key
	s.broadcastLocked()
	s.mu.Unlock()
	return nil
}

func (s *APIKeyService) Delete(ctx context.Context) error {
	if s == nil || s.store == nil {
		return ErrAPIKeyStoreUnavailable
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if err := s.store.DeleteAgentAPIKey(ctx, s.id); err != nil {
		return err
	}
	s.mu.Lock()
	s.key = ""
	s.broadcastLocked()
	s.mu.Unlock()
	return nil
}

func (s *APIKeyService) broadcastLocked() {
	status := APIKeyStatus{Authenticated: s.key != ""}
	for ch := range s.subs {
		select {
		case ch <- status:
		default:
			delete(s.subs, ch)
			close(ch)
		}
	}
}
