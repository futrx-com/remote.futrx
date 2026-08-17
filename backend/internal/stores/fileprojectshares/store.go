package fileprojectshares

// File-backed storage for per-project public preview links. One JSON file per
// project at <dataDir>/projectshares/<projectID>.json, mode 0600, holding the
// link records plus an updatedAt unix-ms stamp. Atomic write via temp+rename,
// one mutex per project.
//
// Only the SHA-256 digest of each token is stored, so this file cannot be
// turned back into a working preview link.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceshare "github.com/futrx-com/remote.futrx.com/internal/service/share"
)

var _ serviceshare.Repository = (*Store)(nil)

type sharesFile struct {
	Shares    []serviceshare.Share `json:"shares"`
	UpdatedAt int64                `json:"updatedAt"`
}

type Store struct {
	root string

	mu    sync.Mutex
	locks map[serviceproject.ID]*sync.Mutex
}

func New(dataDir string) (*Store, error) {
	dir := filepath.Join(dataDir, "projectshares")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create projectshares dir: %w", err)
	}
	_ = os.Chmod(dir, 0o700)
	return &Store{root: dir, locks: map[serviceproject.ID]*sync.Mutex{}}, nil
}

func (s *Store) path(id serviceproject.ID) string {
	return filepath.Join(s.root, string(id)+".json")
}

func (s *Store) lock(id serviceproject.ID) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.locks[id]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.locks[id] = m
	return m
}

func (s *Store) List(_ context.Context, projectID serviceproject.ID) ([]serviceshare.Share, error) {
	mu := s.lock(projectID)
	mu.Lock()
	defer mu.Unlock()
	return s.loadLocked(projectID)
}

func (s *Store) Update(
	_ context.Context,
	projectID serviceproject.ID,
	fn func([]serviceshare.Share) ([]serviceshare.Share, error),
) ([]serviceshare.Share, error) {
	if fn == nil {
		return nil, errors.New("share update function is required")
	}
	mu := s.lock(projectID)
	mu.Lock()
	defer mu.Unlock()

	current, err := s.loadLocked(projectID)
	if err != nil {
		return nil, err
	}
	next, err := fn(current)
	if err != nil {
		return nil, err
	}
	if err := s.saveLocked(projectID, next); err != nil {
		return nil, err
	}
	return next, nil
}

func (s *Store) loadLocked(id serviceproject.ID) ([]serviceshare.Share, error) {
	raw, err := os.ReadFile(s.path(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var file sharesFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse shares for %s: %w", id, err)
	}
	return file.Shares, nil
}

func (s *Store) saveLocked(id serviceproject.ID, shares []serviceshare.Share) error {
	if len(shares) == 0 {
		if err := os.Remove(s.path(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	out := sharesFile{Shares: shares, UpdatedAt: time.Now().UnixMilli()}

	tmp, err := os.CreateTemp(s.root, "."+string(id)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path(id))
}
