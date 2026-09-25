package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

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

// APIKeyService owns one provider's stored API-key accounts and broadcasts
// configured/unconfigured transitions to auth subscribers.
type APIKeyService struct {
	id            agent.ProviderID
	store         APIKeyStore
	validator     APIKeyValidator
	accountsStore AccountStore
	accountLabel  string

	mutationMu sync.Mutex
	mu         sync.RWMutex
	key        string
	accounts   AccountSet
	subs       map[chan APIKeyStatus]struct{}
}

// EnableAccounts promotes this service from one replaceable key to named API
// key accounts backed by the shared private account store. A legacy
// key is migrated as the active "Default" account on first use.
func (s *APIKeyService) EnableAccounts(ctx context.Context, vault *AccountVault, providerLabel string) error {
	if s == nil || vault == nil || vault.store == nil {
		return nil
	}
	providerLabel = strings.TrimSpace(providerLabel)
	if providerLabel == "" {
		return errors.New("API key accounts need a provider label")
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	accounts, err := vault.store.AgentAccounts(ctx, s.id)
	if err != nil {
		return err
	}

	s.mu.Lock()
	legacyKey := s.key
	s.mu.Unlock()
	if len(accounts.Accounts) == 0 && legacyKey != "" {
		credential, marshalErr := json.Marshal(legacyKey)
		if marshalErr != nil {
			return marshalErr
		}
		id, idErr := newAccountID()
		if idErr != nil {
			return idErr
		}
		accounts = AccountSet{
			ActiveAccountID: id,
			Accounts: []AccountRecord{{
				ID: id, Label: "Default", ValidatedAt: time.Now().UTC(), Credential: credential,
			}},
		}
		if err := vault.store.SaveAgentAccounts(ctx, s.id, accounts); err != nil {
			return err
		}
	}
	// Once a usable account-set copy exists, the legacy singleton is only a
	// duplicate secret. Retry its removal on every startup in case an earlier
	// migration committed the account set but could not clean up the old file.
	if legacyKey != "" && s.store != nil {
		if _, ok := apiKeyFromAccounts(accounts, ""); ok {
			if err := s.store.DeleteAgentAPIKey(ctx, s.id); err != nil {
				log.Printf("%s API key accounts: remove migrated legacy key: %v", s.id, err)
			}
		}
	}
	s.mu.Lock()
	s.accountsStore = vault.store
	s.accountLabel = providerLabel
	s.accounts = accounts
	s.key = ""
	s.mu.Unlock()
	return nil
}

func (s *APIKeyService) AccountsEnabled() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.accountsStore != nil
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
	if s.accountsStore != nil {
		_, ok := apiKeyFromAccounts(s.accounts, "")
		return ok
	}
	return s.key != ""
}

func (s *APIKeyService) APIKey() (string, bool) {
	if s == nil {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.accountsStore != nil {
		return apiKeyFromAccounts(s.accounts, "")
	}
	return s.key, s.key != ""
}

// APIKeyFor resolves accountID to an API key without changing global state.
// Empty accountID uses the most recently activated account.
func (s *APIKeyService) APIKeyFor(accountID string) (string, bool) {
	if s == nil {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.accountsStore == nil {
		return s.key, s.key != "" && strings.TrimSpace(accountID) == ""
	}
	return apiKeyFromAccounts(s.accounts, strings.TrimSpace(accountID))
}

func apiKeyFromAccounts(accounts AccountSet, accountID string) (string, bool) {
	if accountID == "" {
		accountID = accounts.ActiveAccountID
	}
	record, ok := accounts.Find(accountID)
	if !ok {
		return "", false
	}
	var key string
	if err := json.Unmarshal(record.Credential, &key); err != nil {
		return "", false
	}
	key = strings.TrimSpace(key)
	return key, key != ""
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
	authenticated := s.key != ""
	if s.accountsStore != nil {
		_, authenticated = apiKeyFromAccounts(s.accounts, "")
	}
	status := APIKeyStatus{Authenticated: authenticated}
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
	if s != nil && s.AccountsEnabled() {
		s.mu.RLock()
		active := s.accounts.ActiveAccountID
		label := ""
		if active == "" {
			label = s.accountLabel
		}
		s.mu.RUnlock()
		return s.SetAccount(ctx, label, active, key)
	}
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

// SetAccount validates and saves a named API key account. accountID replaces
// an existing account; an empty ID creates and activates a new one.
func (s *APIKeyService) SetAccount(ctx context.Context, label, accountID, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return ErrAPIKeyRequired
	}
	if s == nil || !s.AccountsEnabled() {
		return ErrAPIKeyStoreUnavailable
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if s.validator != nil {
		if err := s.validator.ValidateAPIKey(ctx, key); err != nil {
			return err
		}
	}
	credential, err := json.Marshal(key)
	if err != nil {
		return err
	}

	s.mu.RLock()
	next := s.accounts.Clone()
	store := s.accountsStore
	s.mu.RUnlock()
	if accountID == "" {
		label, err = normalizeAccountLabel(label)
		if err != nil {
			return err
		}
		if err := next.ensureUniqueLabel(label, ""); err != nil {
			return err
		}
		accountID, err = newAccountID()
		if err != nil {
			return err
		}
		next.Accounts = append(next.Accounts, AccountRecord{ID: accountID, Label: label, Credential: credential, ValidatedAt: time.Now().UTC()})
	} else {
		record, ok := next.Find(accountID)
		if !ok {
			return ErrAccountNotFound
		}
		if strings.TrimSpace(label) == "" {
			label = record.Label
		} else if label, err = normalizeAccountLabel(label); err != nil {
			return err
		}
		if err := next.ensureUniqueLabel(label, accountID); err != nil {
			return err
		}
		next.replace(AccountRecord{ID: accountID, Label: label, Credential: credential, ValidatedAt: time.Now().UTC()})
	}
	next.ActiveAccountID = accountID
	if err := store.SaveAgentAccounts(ctx, s.id, next); err != nil {
		return err
	}
	s.mu.Lock()
	s.accounts = next
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
	if s.AccountsEnabled() {
		s.mu.RLock()
		store := s.accountsStore
		s.mu.RUnlock()
		if err := store.SaveAgentAccounts(ctx, s.id, AccountSet{}); err != nil {
			return err
		}
		s.mu.Lock()
		s.accounts = AccountSet{}
		s.broadcastLocked()
		s.mu.Unlock()
		return nil
	}
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
	authenticated := s.key != ""
	if s.accountsStore != nil {
		_, authenticated = apiKeyFromAccounts(s.accounts, "")
	}
	status := APIKeyStatus{Authenticated: authenticated}
	for ch := range s.subs {
		select {
		case ch <- status:
		default:
			delete(s.subs, ch)
			close(ch)
		}
	}
}

func (s *APIKeyService) AccountsSnapshot() AccountsSnapshot {
	if s == nil {
		return AccountsSnapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.accounts.Snapshot()
}

func (s *APIKeyService) ImportCurrent(context.Context, string) error {
	return ErrUnsupportedFlow
}

func (s *APIKeyService) StartAccountLogin(context.Context, string, string) (LoginSnapshot, error) {
	return LoginSnapshot{}, ErrUnsupportedFlow
}

func (s *APIKeyService) ActivateAccount(ctx context.Context, accountID string) error {
	if s == nil || !s.AccountsEnabled() {
		return ErrUnsupportedFlow
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.RLock()
	next := s.accounts.Clone()
	store := s.accountsStore
	s.mu.RUnlock()
	if _, ok := next.Find(accountID); !ok {
		return ErrAccountNotFound
	}
	next.ActiveAccountID = accountID
	if err := store.SaveAgentAccounts(ctx, s.id, next); err != nil {
		return err
	}
	s.mu.Lock()
	s.accounts = next
	s.broadcastLocked()
	s.mu.Unlock()
	return nil
}

func (s *APIKeyService) DeleteAccount(ctx context.Context, accountID string) error {
	if s == nil || !s.AccountsEnabled() {
		return ErrUnsupportedFlow
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.RLock()
	next := s.accounts.Clone()
	store := s.accountsStore
	s.mu.RUnlock()
	if accountID == next.ActiveAccountID {
		return ErrActiveAccountDelete
	}
	if !next.remove(accountID) {
		return ErrAccountNotFound
	}
	if err := store.SaveAgentAccounts(ctx, s.id, next); err != nil {
		return fmt.Errorf("save %s API key accounts: %w", s.id, err)
	}
	s.mu.Lock()
	s.accounts = next
	s.broadcastLocked()
	s.mu.Unlock()
	return nil
}

var _ AccountController = (*APIKeyService)(nil)
