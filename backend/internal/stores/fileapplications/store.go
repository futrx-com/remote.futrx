// Package fileapplications is file-backed storage for installed applications.
//
// Layout under <dataDir>/applications:
//
//	global.json            map[instanceID]Instance   (global-scope apps)
//	projects/<id>.json     map[instanceID]Instance   (that project's apps)
//
// Records may contain generated secrets (DB passwords), so every file is mode
// 0600 and written with temp-file + rename for atomic replacement, mirroring
// fileprojectsecrets.
package fileapplications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

var _ svc.Store = (*Store)(nil)

// Store persists application instances as JSON files.
type Store struct {
	root string

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// New prepares the applications storage directory.
func New(dataDir string) (*Store, error) {
	root := filepath.Join(dataDir, "applications")
	if err := os.MkdirAll(filepath.Join(root, "projects"), 0o700); err != nil {
		return nil, fmt.Errorf("create applications dir: %w", err)
	}
	_ = os.Chmod(root, 0o700)
	_ = os.Chmod(filepath.Join(root, "projects"), 0o700)
	return &Store{root: root, locks: map[string]*sync.Mutex{}}, nil
}

func (s *Store) globalPath() string { return filepath.Join(s.root, "global.json") }

func (s *Store) projectPath(projectID string) string {
	return filepath.Join(s.root, "projects", projectID+".json")
}

func (s *Store) lock(path string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.locks[path]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.locks[path] = m
	return m
}

func loadFile(path string) (map[string]svc.Instance, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]svc.Instance{}, nil
		}
		return nil, err
	}
	out := map[string]svc.Instance{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return out, nil
}

func saveFile(path string, m map[string]svc.Instance) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".apps-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func sorted(m map[string]svc.Instance) []svc.Instance {
	out := make([]svc.Instance, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

// ListGlobal returns all global-scope instances.
func (s *Store) ListGlobal(_ context.Context) ([]svc.Instance, error) {
	path := s.globalPath()
	mu := s.lock(path)
	mu.Lock()
	defer mu.Unlock()
	m, err := loadFile(path)
	if err != nil {
		return nil, err
	}
	return sorted(m), nil
}

// ListProject returns all instances scoped to a project.
func (s *Store) ListProject(_ context.Context, projectID string) ([]svc.Instance, error) {
	path := s.projectPath(projectID)
	mu := s.lock(path)
	mu.Lock()
	defer mu.Unlock()
	m, err := loadFile(path)
	if err != nil {
		return nil, err
	}
	return sorted(m), nil
}

// ListAll returns every instance across global and project storage.
func (s *Store) ListAll(ctx context.Context) ([]svc.Instance, error) {
	out, err := s.ListGlobal(ctx)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(s.root, "projects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		mu := s.lock(path)
		mu.Lock()
		m, err := loadFile(path)
		mu.Unlock()
		if err != nil {
			return nil, err
		}
		out = append(out, sorted(m)...)
	}
	return out, nil
}

// Get finds an instance by ID across global and project storage.
func (s *Store) Get(ctx context.Context, id string) (svc.Instance, bool, error) {
	// Global first.
	if inst, ok, err := s.getFrom(s.globalPath(), id); err != nil || ok {
		return inst, ok, err
	}
	// Then every project file.
	dir := filepath.Join(s.root, "projects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return svc.Instance{}, false, nil
		}
		return svc.Instance{}, false, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		if inst, ok, err := s.getFrom(filepath.Join(dir, e.Name()), id); err != nil || ok {
			return inst, ok, err
		}
	}
	return svc.Instance{}, false, nil
}

func (s *Store) getFrom(path, id string) (svc.Instance, bool, error) {
	mu := s.lock(path)
	mu.Lock()
	defer mu.Unlock()
	m, err := loadFile(path)
	if err != nil {
		return svc.Instance{}, false, err
	}
	inst, ok := m[id]
	return inst, ok, nil
}

// Put writes an instance to the file selected by its scope.
func (s *Store) Put(_ context.Context, inst svc.Instance) error {
	path := s.pathFor(inst)
	mu := s.lock(path)
	mu.Lock()
	defer mu.Unlock()
	m, err := loadFile(path)
	if err != nil {
		return err
	}
	m[inst.ID] = inst
	return saveFile(path, m)
}

// Delete removes an instance by ID from whichever file holds it.
func (s *Store) Delete(_ context.Context, id string) error {
	paths := []string{s.globalPath()}
	dir := filepath.Join(s.root, "projects")
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
				paths = append(paths, filepath.Join(dir, e.Name()))
			}
		}
	}
	for _, path := range paths {
		mu := s.lock(path)
		mu.Lock()
		m, err := loadFile(path)
		if err != nil {
			mu.Unlock()
			return err
		}
		if _, ok := m[id]; ok {
			delete(m, id)
			err = saveFile(path, m)
			mu.Unlock()
			return err
		}
		mu.Unlock()
	}
	return nil
}

func (s *Store) pathFor(inst svc.Instance) string {
	if inst.Scope == svc.ScopeProject && inst.ProjectID != "" {
		return s.projectPath(inst.ProjectID)
	}
	return s.globalPath()
}
