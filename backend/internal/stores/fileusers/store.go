package fileusers

// File-backed storage for registered users. Single JSON file at
// <dataDir>/users.json, mode 0600. Wraps writes in temp+rename for atomicity.
// All emails normalized (lowercased, trimmed) before persisting.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/futrx-com/remote.futrx.com/internal/service/user"
)

var _ user.Repository = (*Store)(nil)

type usersFile struct {
	Users []user.User `json:"users"`
}

type Store struct {
	dir  string
	path string

	mu sync.Mutex
}

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &Store{
		dir:  dataDir,
		path: filepath.Join(dataDir, "users.json"),
	}, nil
}

func (s *Store) loadLocked() (map[string]user.User, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]user.User{}, nil
		}
		return nil, err
	}
	var f usersFile
	if len(raw) == 0 {
		return map[string]user.User{}, nil
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse users.json: %w", err)
	}
	m := make(map[string]user.User, len(f.Users))
	for _, u := range f.Users {
		em := user.NormalizeEmail(u.Email)
		if em == "" {
			continue
		}
		u.Email = em
		u.Role = user.NormalizeRole(string(u.Role))
		m[em] = u
	}
	return m, nil
}

func (s *Store) saveLocked(m map[string]user.User) error {
	out := usersFile{Users: make([]user.User, 0, len(m))}
	for _, u := range m {
		out.Users = append(out.Users, u)
	}
	sort.Slice(out.Users, func(i, j int) bool {
		return out.Users[i].Email < out.Users[j].Email
	})

	tmp, err := os.CreateTemp(s.dir, ".users-*.json.tmp")
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
	return os.Rename(tmpName, s.path)
}

func (s *Store) List(_ context.Context) ([]user.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	out := make([]user.User, 0, len(m))
	for _, u := range m {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out, nil
}

func (s *Store) Get(_ context.Context, email string) (*user.User, error) {
	em := user.NormalizeEmail(email)
	if em == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	u, ok := m[em]
	if !ok {
		return nil, nil
	}
	return &u, nil
}

func (s *Store) Add(_ context.Context, u user.User) error {
	em := user.NormalizeEmail(u.Email)
	if em == "" {
		return user.ErrInvalidEmail
	}
	u.Email = em
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.loadLocked()
	if err != nil {
		return err
	}
	if _, exists := m[em]; exists {
		return user.ErrUserExists
	}
	m[em] = u
	return s.saveLocked(m)
}

func (s *Store) Remove(_ context.Context, email string) error {
	em := user.NormalizeEmail(email)
	if em == "" {
		return user.ErrInvalidEmail
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.loadLocked()
	if err != nil {
		return err
	}
	if _, ok := m[em]; !ok {
		return user.ErrUserNotFound
	}
	delete(m, em)
	return s.saveLocked(m)
}

func (s *Store) SetRole(_ context.Context, email string, role user.Role) error {
	em := user.NormalizeEmail(email)
	if em == "" {
		return user.ErrInvalidEmail
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.loadLocked()
	if err != nil {
		return err
	}
	u, ok := m[em]
	if !ok {
		return user.ErrUserNotFound
	}
	u.Role = role
	m[em] = u
	return s.saveLocked(m)
}

func (s *Store) Count(_ context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.loadLocked()
	if err != nil {
		return 0, err
	}
	return len(m), nil
}
