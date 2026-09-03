package fileauth

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

const agentAPIKeysFile = "agent-api-keys.json"

type Store struct {
	dataDir string
	mu      sync.Mutex
}

func New(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

func (s *Store) OAuthConfig(ctx context.Context) (serviceauth.OAuthConfig, error) {
	select {
	case <-ctx.Done():
		return serviceauth.OAuthConfig{}, ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.oauthConfigLocked()
}

func (s *Store) oauthConfigLocked() (serviceauth.OAuthConfig, error) {
	data, err := os.ReadFile(filepath.Join(s.dataDir, "oauth.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return serviceauth.OAuthConfig{}, serviceauth.ErrOAuthConfigNotFound
		}
		return serviceauth.OAuthConfig{}, fmt.Errorf("read oauth.json: %w", err)
	}
	var cfg serviceauth.OAuthConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return serviceauth.OAuthConfig{}, fmt.Errorf("parse oauth.json: %w", err)
	}
	if cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
		return serviceauth.OAuthConfig{}, errors.New("oauth.json present but missing googleClientId or googleClientSecret")
	}
	return cfg, nil
}

func (s *Store) SaveOAuthConfig(ctx context.Context, cfg serviceauth.OAuthConfig) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	cfg.GoogleClientID = strings.TrimSpace(cfg.GoogleClientID)
	cfg.GoogleClientSecret = strings.TrimSpace(cfg.GoogleClientSecret)
	if cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
		return serviceauth.ErrInvalidOAuthConfig
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSONLocked("oauth.json", cfg)
}

func (s *Store) AgentAPIKey(ctx context.Context, provider agent.ProviderID) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	if provider == "" {
		return "", errors.New("agent provider is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	keys, err := s.agentAPIKeysLocked()
	if err != nil {
		return "", err
	}
	return keys[string(provider)], nil
}

func (s *Store) SaveAgentAPIKey(ctx context.Context, provider agent.ProviderID, key string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	key = strings.TrimSpace(key)
	if provider == "" || key == "" {
		return errors.New("agent provider and API key are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	keys, err := s.agentAPIKeysLocked()
	if err != nil {
		return err
	}
	keys[string(provider)] = key
	return s.writeJSONLocked(agentAPIKeysFile, keys)
}

func (s *Store) DeleteAgentAPIKey(ctx context.Context, provider agent.ProviderID) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if provider == "" {
		return errors.New("agent provider is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	keys, err := s.agentAPIKeysLocked()
	if err != nil {
		return err
	}
	delete(keys, string(provider))
	return s.writeJSONLocked(agentAPIKeysFile, keys)
}

func (s *Store) agentAPIKeysLocked() (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(s.dataDir, agentAPIKeysFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("read %s: %w", agentAPIKeysFile, err)
	}
	keys := make(map[string]string)
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("parse %s: %w", agentAPIKeysFile, err)
	}
	return keys, nil
}

func (s *Store) LocalAdmin(ctx context.Context) (*serviceauth.LocalAdminCredential, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.localAdminLocked()
}

func (s *Store) localAdminLocked() (*serviceauth.LocalAdminCredential, error) {
	data, err := os.ReadFile(filepath.Join(s.dataDir, "local-admin.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read local-admin.json: %w", err)
	}
	var credential serviceauth.LocalAdminCredential
	if err := json.Unmarshal(data, &credential); err != nil {
		return nil, fmt.Errorf("parse local-admin.json: %w", err)
	}
	credential.Email = strings.ToLower(strings.TrimSpace(credential.Email))
	if credential.Email == "" || credential.PasswordHash == "" {
		return nil, errors.New("local-admin.json is incomplete")
	}
	return &credential, nil
}

func (s *Store) CreateLocalAdmin(ctx context.Context, credential serviceauth.LocalAdminCredential) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	credential.Email = strings.ToLower(strings.TrimSpace(credential.Email))
	if credential.Email == "" || credential.PasswordHash == "" {
		return errors.New("local admin credential is incomplete")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(filepath.Join(s.dataDir, "local-admin.json")); err == nil {
		return serviceauth.ErrLocalAdminAlreadyClaimed
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.writeJSONLocked("local-admin.json", credential)
}

func (s *Store) DeleteLocalAdmin(ctx context.Context, expected serviceauth.LocalAdminCredential) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	expected.Email = strings.ToLower(strings.TrimSpace(expected.Email))
	if expected.Email == "" || expected.PasswordHash == "" {
		return errors.New("expected local admin credential is incomplete")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.localAdminLocked()
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	if current.Email != expected.Email || current.PasswordHash != expected.PasswordHash {
		return serviceauth.ErrLocalAdminCredentialChanged
	}
	if err := os.Remove(filepath.Join(s.dataDir, "local-admin.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete local-admin.json: %w", err)
	}
	return nil
}

func (s *Store) SessionKey(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dataDir, 0o700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(s.dataDir, "session.key")
	sessionKey, err := os.ReadFile(keyPath)
	if err == nil {
		return sessionKey, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read session.key: %w", err)
	}

	sessionKey = make([]byte, 32)
	if _, err := crand.Read(sessionKey); err != nil {
		return nil, fmt.Errorf("gen session key: %w", err)
	}
	if err := os.WriteFile(keyPath, sessionKey, 0o600); err != nil {
		return nil, fmt.Errorf("write session.key: %w", err)
	}
	return sessionKey, nil
}

func (s *Store) writeJSONLocked(name string, value any) error {
	if err := os.MkdirAll(s.dataDir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dataDir, ".auth-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(s.dataDir, name))
}
