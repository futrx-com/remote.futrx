package filenotify

// File-backed storage for the single global notification configuration at
// <dataDir>/notifications.json, mode 0600 because the document holds a
// Telegram bot token and a webhook shared secret. Writes rename a temp file
// into place for atomic replacement.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	servicenotify "github.com/futrx-com/remote.futrx.com/internal/service/notify"
)

var _ servicenotify.Store = (*Store)(nil)

const fileName = "notifications.json"

type Store struct {
	root string
	mu   sync.Mutex
}

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &Store{root: dataDir}, nil
}

func (s *Store) path() string {
	return filepath.Join(s.root, fileName)
}

// Load returns the stored configuration, or defaults when nothing has been
// saved yet.
func (s *Store) Load(ctx context.Context) (servicenotify.Config, error) {
	select {
	case <-ctx.Done():
		return servicenotify.Config{}, ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return servicenotify.DefaultConfig(), nil
		}
		return servicenotify.Config{}, fmt.Errorf("read notification settings: %w", err)
	}
	if len(raw) == 0 {
		return servicenotify.DefaultConfig(), nil
	}

	cfg := servicenotify.DefaultConfig()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return servicenotify.Config{}, fmt.Errorf("parse notification settings: %w", err)
	}
	return cfg.Normalize(), nil
}

func (s *Store) Save(ctx context.Context, cfg servicenotify.Config) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	tmp, err := os.CreateTemp(s.root, ".notifications-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp notification settings: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("secure temp notification settings: %w", err)
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg.Normalize()); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp notification settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp notification settings: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path()); err != nil {
		return fmt.Errorf("replace notification settings: %w", err)
	}
	return nil
}
