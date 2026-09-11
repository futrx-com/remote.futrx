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
	// index remembers which file each instance id was last seen in. Without it
	// every lookup by id — which is what start, stop and uninstall all begin
	// with — reads global.json and then one file per project until it finds the
	// one it wants, so the cost of touching a single app grows with the number
	// of projects on the server. It is a hint, never an answer: the file it
	// points at is still read and still has to contain the id, and a miss falls
	// back to the full scan that rebuilds it.
	index map[string]string
}

// New prepares the applications storage directory.
func New(dataDir string) (*Store, error) {
	root := filepath.Join(dataDir, "applications")
	if err := os.MkdirAll(filepath.Join(root, "projects"), 0o700); err != nil {
		return nil, fmt.Errorf("create applications dir: %w", err)
	}
	_ = os.Chmod(root, 0o700)
	_ = os.Chmod(filepath.Join(root, "projects"), 0o700)
	return &Store{
		root:  root,
		locks: map[string]*sync.Mutex{},
		index: map[string]string{},
	}, nil
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

// hint returns the file an id was last seen in, if any.
func (s *Store) hint(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, ok := s.index[id]
	return path, ok
}

// remember records where a set of instances lives, so the next lookup of any of
// them reads one file.
func (s *Store) remember(path string, m map[string]svc.Instance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range m {
		s.index[id] = path
	}
}

func (s *Store) forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.index, id)
}

// files lists every file that can hold an instance, global storage first.
func (s *Store) files() ([]string, error) {
	paths := []string{s.globalPath()}
	entries, err := os.ReadDir(filepath.Join(s.root, "projects"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return paths, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		paths = append(paths, filepath.Join(s.root, "projects", e.Name()))
	}
	return paths, nil
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
	s.remember(path, m)
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
	s.remember(path, m)
	return sorted(m), nil
}

// ListAll returns every instance across global and project storage.
func (s *Store) ListAll(_ context.Context) ([]svc.Instance, error) {
	paths, err := s.files()
	if err != nil {
		return nil, err
	}
	var out []svc.Instance
	for _, path := range paths {
		mu := s.lock(path)
		mu.Lock()
		m, err := loadFile(path)
		mu.Unlock()
		if err != nil {
			return nil, err
		}
		s.remember(path, m)
		out = append(out, sorted(m)...)
	}
	return out, nil
}

// Get finds an instance by ID across global and project storage.
func (s *Store) Get(_ context.Context, id string) (svc.Instance, bool, error) {
	if path, ok := s.hint(id); ok {
		if inst, found, err := s.getFrom(path, id); err != nil || found {
			return inst, found, err
		}
	}
	// No hint, or a stale one: read every file, which also repairs the index.
	paths, err := s.files()
	if err != nil {
		return svc.Instance{}, false, err
	}
	for _, path := range paths {
		if inst, ok, err := s.getFrom(path, id); err != nil || ok {
			return inst, ok, err
		}
	}
	s.forget(id)
	return svc.Instance{}, false, nil
}

func (s *Store) getFrom(path, id string) (svc.Instance, bool, error) {
	mu := s.lock(path)
	mu.Lock()
	m, err := loadFile(path)
	mu.Unlock()
	if err != nil {
		return svc.Instance{}, false, err
	}
	s.remember(path, m)
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
	if err := saveFile(path, m); err != nil {
		return err
	}
	s.remember(path, m)
	return nil
}

// Delete removes an instance by ID from whichever file holds it.
func (s *Store) Delete(_ context.Context, id string) error {
	paths, err := s.files()
	if err != nil {
		return err
	}
	// The hint is tried first for the same reason Get tries it, and the full
	// list still follows it: an id that is not where the index says it is has to
	// be deleted from wherever it actually is.
	if path, ok := s.hint(id); ok {
		paths = append([]string{path}, paths...)
	}
	for _, path := range paths {
		deleted, err := s.deleteFrom(path, id)
		if err != nil {
			return err
		}
		if deleted {
			s.forget(id)
			return nil
		}
	}
	s.forget(id)
	return nil
}

func (s *Store) deleteFrom(path, id string) (bool, error) {
	mu := s.lock(path)
	mu.Lock()
	defer mu.Unlock()
	m, err := loadFile(path)
	if err != nil {
		return false, err
	}
	if _, ok := m[id]; !ok {
		return false, nil
	}
	delete(m, id)
	return true, saveFile(path, m)
}

func (s *Store) pathFor(inst svc.Instance) string {
	if inst.Scope == svc.ScopeProject && inst.ProjectID != "" {
		return s.projectPath(inst.ProjectID)
	}
	return s.globalPath()
}
