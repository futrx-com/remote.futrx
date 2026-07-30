package filecustomprovider

// File-backed storage for the admin-supplied custom AI provider. A single
// JSON file at <dataDir>/custom-provider.json, mode 0600, holds the display
// name, API key, and base URL. Write path renames a temp file into place for
// atomic replacement. The API key is persisted here because it is needed at
// run time; it is never surfaced by the auth service's status responses.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

var _ agentauth.APIKeyStore = (*Store)(nil)

type Store struct {
	path string
	mu   sync.Mutex
}

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create custom provider dir: %w", err)
	}
	return &Store{path: filepath.Join(dataDir, "custom-provider.json")}, nil
}

func (s *Store) Load() (agentauth.APIKeyConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return agentauth.APIKeyConfig{}, nil
		}
		return agentauth.APIKeyConfig{}, err
	}
	if len(raw) == 0 {
		return agentauth.APIKeyConfig{}, nil
	}
	var cfg agentauth.APIKeyConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return agentauth.APIKeyConfig{}, fmt.Errorf("parse custom provider: %w", err)
	}
	return cfg, nil
}

func (s *Store) Save(cfg agentauth.APIKeyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create custom provider dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".custom-provider-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}
