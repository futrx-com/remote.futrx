package fileproject

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

const WorkspaceRoot = "/var/lib/remote/projects"

var _ serviceproject.Repository = (*Store)(nil)

type Store struct {
	root          string
	workspaceRoot string
	mu            sync.Mutex
	locks         map[serviceproject.ID]*sync.Mutex
	indexMu       sync.RWMutex
	bySlug        map[string]serviceproject.ID
}

func New(dataDir string) (*Store, error) {
	return NewWithWorkspaceRoot(dataDir, WorkspaceRoot)
}

func NewWithWorkspaceRoot(dataDir, workspaceRoot string) (*Store, error) {
	dir := filepath.Join(dataDir, "projects")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create projects dir: %w", err)
	}
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace root: %w", err)
	}
	s := &Store{
		root:          dir,
		workspaceRoot: workspaceRoot,
		locks:         map[serviceproject.ID]*sync.Mutex{},
		bySlug:        map[string]serviceproject.ID{},
	}
	if err := s.loadSlugIndex(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) projectDir(id serviceproject.ID) string {
	return filepath.Join(s.root, string(id))
}

func (s *Store) WorkspaceDir(slug string) string {
	return filepath.Join(s.workspaceRoot, slug, "workspace")
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

func newProjectID() serviceproject.ID {
	var b [6]byte
	_, _ = crand.Read(b[:])
	return serviceproject.ID(hex.EncodeToString(b[:]))
}

func (s *Store) Create(ctx context.Context, m serviceproject.Meta) (serviceproject.Meta, error) {
	if strings.TrimSpace(m.Name) == "" {
		return serviceproject.Meta{}, serviceproject.ErrNameRequired
	}
	base := m.Slug
	if base == "" {
		base = serviceproject.Slugify(m.Name)
	}
	if !serviceproject.ValidSlug(base) {
		base = serviceproject.Slugify(m.Name)
	}

	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	taken, err := s.nameTakenLocked(m.Name, "")
	if err != nil {
		return serviceproject.Meta{}, err
	}
	if taken {
		return serviceproject.Meta{}, serviceproject.ErrNameAlreadyExists
	}

	slug, err := s.resolveSlugLocked(base)
	if err != nil {
		return serviceproject.Meta{}, err
	}
	now := time.Now().UnixMilli()
	m.ID = newProjectID()
	m.Slug = slug
	m.Cwd = s.WorkspaceDir(slug)
	m.ContainerName = slug
	if m.Status == "" {
		m.Status = serviceproject.StatusProvisioning
	}
	if m.CreatedAt == 0 {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	if m.Order == 0 {
		m.Order = now
	}

	if err := s.writeMeta(m); err != nil {
		return serviceproject.Meta{}, err
	}
	s.bySlug[slug] = m.ID
	return m, nil
}

func (s *Store) Get(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error) {
	return s.readMeta(id)
}

func (s *Store) GetBySlug(ctx context.Context, slug string) (serviceproject.Meta, error) {
	s.indexMu.RLock()
	id, ok := s.bySlug[slug]
	s.indexMu.RUnlock()
	if !ok {
		return serviceproject.Meta{}, serviceproject.ErrNotFound
	}
	return s.Get(ctx, id)
}

func (s *Store) List(ctx context.Context) ([]serviceproject.Meta, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	out := make([]serviceproject.Meta, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := s.readMeta(serviceproject.ID(e.Name()))
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		left := projectOrder(out[i])
		right := projectOrder(out[j])
		if left == right {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return left > right
	})
	return out, nil
}

func projectOrder(m serviceproject.Meta) int64 {
	if m.Order != 0 {
		return m.Order
	}
	return m.CreatedAt
}

func (s *Store) Update(
	ctx context.Context,
	id serviceproject.ID,
	fn func(*serviceproject.Meta),
) (serviceproject.Meta, error) {
	lk := s.lock(id)
	lk.Lock()
	defer lk.Unlock()
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	m, err := s.readMeta(id)
	if err != nil {
		return serviceproject.Meta{}, err
	}
	previousName := m.Name
	fn(&m)
	if !sameProjectName(previousName, m.Name) {
		taken, err := s.nameTakenLocked(m.Name, id)
		if err != nil {
			return serviceproject.Meta{}, err
		}
		if taken {
			return serviceproject.Meta{}, serviceproject.ErrNameAlreadyExists
		}
	}
	m.UpdatedAt = time.Now().UnixMilli()
	if err := s.writeMeta(m); err != nil {
		return serviceproject.Meta{}, err
	}
	return m, nil
}

func (s *Store) SetStatus(
	ctx context.Context,
	id serviceproject.ID,
	status serviceproject.Status,
	errMsg string,
) (serviceproject.Meta, error) {
	return s.Update(ctx, id, func(m *serviceproject.Meta) {
		m.Status = status
		m.ErrorMsg = errMsg
	})
}

func (s *Store) Delete(ctx context.Context, id serviceproject.ID) error {
	lk := s.lock(id)
	lk.Lock()
	defer lk.Unlock()

	m, err := s.readMeta(id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(s.projectDir(id)); err != nil {
		return fmt.Errorf("remove project meta dir: %w", err)
	}
	if m.Cwd != "" {
		_ = os.RemoveAll(filepath.Dir(m.Cwd))
	}
	s.indexMu.Lock()
	delete(s.bySlug, m.Slug)
	s.indexMu.Unlock()
	s.mu.Lock()
	delete(s.locks, id)
	s.mu.Unlock()
	return nil
}

func (s *Store) readMeta(id serviceproject.ID) (serviceproject.Meta, error) {
	if !serviceproject.ValidID(id) {
		return serviceproject.Meta{}, serviceproject.ErrInvalidID
	}
	data, err := os.ReadFile(filepath.Join(s.projectDir(id), "meta.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return serviceproject.Meta{}, serviceproject.ErrNotFound
		}
		return serviceproject.Meta{}, err
	}
	var m serviceproject.Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return serviceproject.Meta{}, fmt.Errorf("parse meta for %s: %w", id, err)
	}
	return m, nil
}

func (s *Store) writeMeta(m serviceproject.Meta) error {
	dir := s.projectDir(m.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".meta.json.tmp")
	final := filepath.Join(dir, "meta.json")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func (s *Store) loadSlugIndex() error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := s.readMeta(serviceproject.ID(e.Name()))
		if err != nil {
			continue
		}
		s.bySlug[m.Slug] = m.ID
	}
	return nil
}

func (s *Store) resolveSlugLocked(base string) (string, error) {
	if _, taken := s.bySlug[base]; !taken {
		return base, nil
	}
	for i := 2; i < 1000; i++ {
		suffix := fmt.Sprintf("-%d", i)
		stem := base
		if len(stem)+len(suffix) > serviceproject.MaxSlugLen {
			stem = stem[:serviceproject.MaxSlugLen-len(suffix)]
		}
		cand := stem + suffix
		if _, taken := s.bySlug[cand]; !taken {
			return cand, nil
		}
	}
	return "", errors.New("could not find an available slug")
}

func (s *Store) nameTakenLocked(name string, except serviceproject.ID) (bool, error) {
	for _, id := range s.bySlug {
		if id == except {
			continue
		}
		m, err := s.readMeta(id)
		if err != nil {
			// Delete removes project metadata before releasing its slug from the
			// index. Treat that short-lived stale entry as already deleted.
			if errors.Is(err, serviceproject.ErrNotFound) {
				continue
			}
			return false, err
		}
		if sameProjectName(m.Name, name) {
			return true, nil
		}
	}
	return false, nil
}

func sameProjectName(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}
