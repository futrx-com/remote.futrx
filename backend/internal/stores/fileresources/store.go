// Package fileresources persists the fleet resource policy as a single JSON
// document at DATA_DIR/resources.json. It follows the same temp-file + rename
// discipline as the other metadata stores, and reports "absent" rather than an
// error so the service can derive host-aware defaults on first run.
package fileresources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	serviceresources "github.com/futrx-com/remote.futrx.com/internal/service/resources"
)

var _ serviceresources.Repository = (*Store)(nil)

// FileName is the policy document's name inside DATA_DIR. Exported so the
// operations docs and any recovery tooling agree on one spelling.
const FileName = "resources.json"

type Store struct {
	path string
	mu   sync.Mutex
}

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &Store{path: filepath.Join(dataDir, FileName)}, nil
}

func (s *Store) Load(ctx context.Context) (serviceresources.Settings, bool, error) {
	select {
	case <-ctx.Done():
		return serviceresources.Settings{}, false, ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return serviceresources.Settings{}, false, nil
	}
	if err != nil {
		return serviceresources.Settings{}, false, fmt.Errorf("read resource settings: %w", err)
	}
	var settings serviceresources.Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return serviceresources.Settings{}, false, fmt.Errorf("parse resource settings: %w", err)
	}
	return settings, true, nil
}

func (s *Store) Save(ctx context.Context, settings serviceresources.Settings) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal resource settings: %w", err)
	}
	data = append(data, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "resources-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp resource settings: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp resource settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp resource settings: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("chmod temp resource settings: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace resource settings: %w", err)
	}
	return nil
}
