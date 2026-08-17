// Package fileskillsglobal persists the platform-wide global skills library
// as ordinary skill directories under DATA_DIR/skills-global, so the on-disk
// format matches the project skills the agent CLIs already read.
package fileskillsglobal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	serviceskills "github.com/futrx-com/remote.futrx.com/internal/service/skills"
)

var _ serviceskills.GlobalRepository = (*Store)(nil)

// DirName is the DATA_DIR child that holds the global skills library.
const DirName = "skills-global"

// indexFileName holds the admin policy flags that have no place inside a
// portable SKILL.md directory (currently the "always on" bit).
const indexFileName = "_index.json"

// Dir returns the global skills library root for a data directory. The
// container provisioner reads the same path, so both sides agree on layout.
func Dir(dataDir string) string {
	return filepath.Join(dataDir, DirName)
}

// Store is a file-backed global skills library guarded by an in-process mutex,
// matching the concurrency model of the other DATA_DIR stores.
type Store struct {
	root string
	mu   sync.Mutex
}

// New creates the library directory and returns a store rooted at it.
func New(dataDir string) (*Store, error) {
	root := Dir(dataDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create global skills dir: %w", err)
	}
	return &Store{root: root}, nil
}

type indexFile struct {
	Skills map[string]indexEntry `json:"skills"`
}

type indexEntry struct {
	AlwaysOn  bool  `json:"alwaysOn,omitempty"`
	UpdatedAt int64 `json:"updatedAt,omitempty"`
}

// List returns every skill directory with its policy flags and file names.
// File contents are omitted: the admin list view never needs them.
func (s *Store) List(ctx context.Context) ([]serviceskills.GlobalRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	names, err := s.skillNames()
	if err != nil {
		return nil, err
	}
	index, err := s.readIndex()
	if err != nil {
		return nil, err
	}

	records := make([]serviceskills.GlobalRecord, 0, len(names))
	for _, name := range names {
		record, err := s.read(name, index, false)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// Get returns one skill including the contents of every file it owns.
func (s *Store) Get(ctx context.Context, name string) (serviceskills.GlobalRecord, error) {
	if err := ctx.Err(); err != nil {
		return serviceskills.GlobalRecord{}, err
	}
	if !serviceskills.ValidGlobalSkillName(name) {
		return serviceskills.GlobalRecord{}, serviceskills.ErrInvalidGlobalSkillName
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.requireSkill(name); err != nil {
		return serviceskills.GlobalRecord{}, err
	}
	index, err := s.readIndex()
	if err != nil {
		return serviceskills.GlobalRecord{}, err
	}
	return s.read(name, index, true)
}

// Save replaces the skill directory with the supplied file set and records its
// policy flags. Content is staged in a sibling directory and renamed into
// place so a failed write never leaves a half-written skill behind.
func (s *Store) Save(
	ctx context.Context,
	record serviceskills.GlobalRecord,
) (serviceskills.GlobalRecord, error) {
	if err := ctx.Err(); err != nil {
		return serviceskills.GlobalRecord{}, err
	}
	if !serviceskills.ValidGlobalSkillName(record.Name) {
		return serviceskills.GlobalRecord{}, serviceskills.ErrInvalidGlobalSkillName
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	target := filepath.Join(s.root, record.Name)
	staging, err := os.MkdirTemp(s.root, ".staging-*")
	if err != nil {
		return serviceskills.GlobalRecord{}, fmt.Errorf("stage global skill: %w", err)
	}
	defer os.RemoveAll(staging)

	for _, relative := range sortedKeys(record.Files) {
		destination := filepath.Join(staging, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return serviceskills.GlobalRecord{}, fmt.Errorf("create global skill dir: %w", err)
		}
		if err := os.WriteFile(destination, record.Files[relative], 0o644); err != nil {
			return serviceskills.GlobalRecord{}, fmt.Errorf("write global skill file: %w", err)
		}
	}

	if err := os.RemoveAll(target); err != nil {
		return serviceskills.GlobalRecord{}, fmt.Errorf("replace global skill: %w", err)
	}
	if err := os.Rename(staging, target); err != nil {
		return serviceskills.GlobalRecord{}, fmt.Errorf("install global skill: %w", err)
	}

	updatedAt := record.UpdatedAt
	if updatedAt == 0 {
		updatedAt = time.Now().UnixMilli()
	}
	index, err := s.readIndex()
	if err != nil {
		return serviceskills.GlobalRecord{}, err
	}
	index.Skills[record.Name] = indexEntry{AlwaysOn: record.AlwaysOn, UpdatedAt: updatedAt}
	if err := s.writeIndex(index); err != nil {
		return serviceskills.GlobalRecord{}, err
	}
	return s.read(record.Name, index, true)
}

// SetAlwaysOn updates only the admin policy flag of an existing skill.
func (s *Store) SetAlwaysOn(
	ctx context.Context,
	name string,
	alwaysOn bool,
) (serviceskills.GlobalRecord, error) {
	if err := ctx.Err(); err != nil {
		return serviceskills.GlobalRecord{}, err
	}
	if !serviceskills.ValidGlobalSkillName(name) {
		return serviceskills.GlobalRecord{}, serviceskills.ErrInvalidGlobalSkillName
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.requireSkill(name); err != nil {
		return serviceskills.GlobalRecord{}, err
	}
	index, err := s.readIndex()
	if err != nil {
		return serviceskills.GlobalRecord{}, err
	}
	entry := index.Skills[name]
	entry.AlwaysOn = alwaysOn
	if entry.UpdatedAt == 0 {
		entry.UpdatedAt = time.Now().UnixMilli()
	}
	index.Skills[name] = entry
	if err := s.writeIndex(index); err != nil {
		return serviceskills.GlobalRecord{}, err
	}
	return s.read(name, index, false)
}

// Delete removes the skill directory and forgets its policy flags.
func (s *Store) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !serviceskills.ValidGlobalSkillName(name) {
		return serviceskills.ErrInvalidGlobalSkillName
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.requireSkill(name); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(s.root, name)); err != nil {
		return fmt.Errorf("remove global skill: %w", err)
	}
	index, err := s.readIndex()
	if err != nil {
		return err
	}
	delete(index.Skills, name)
	return s.writeIndex(index)
}

// requireSkill reports whether a directory currently holds a readable skill.
// A directory without a SKILL.md is not a skill, so it reads as not found.
func (s *Store) requireSkill(name string) error {
	_, err := os.Stat(filepath.Join(s.root, name, serviceskills.SkillFileName))
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return serviceskills.ErrGlobalSkillNotFound
	}
	return fmt.Errorf("stat global skill: %w", err)
}

// skillNames returns the sorted names of every directory that carries a
// SKILL.md. Reserved children (the index, dotfiles, staging directories) are
// ignored so a half-written skill never appears in the library.
func (s *Store) skillNames() ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read global skills dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !serviceskills.ValidGlobalSkillName(entry.Name()) {
			continue
		}
		if s.requireSkill(entry.Name()) != nil {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func (s *Store) read(name string, index indexFile, withContents bool) (serviceskills.GlobalRecord, error) {
	root := filepath.Join(s.root, name)
	record := serviceskills.GlobalRecord{
		Name:      name,
		AlwaysOn:  index.Skills[name].AlwaysOn,
		UpdatedAt: index.Skills[name].UpdatedAt,
	}
	if withContents {
		record.Files = map[string][]byte{}
	}

	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if current != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || strings.HasPrefix(entry.Name(), ".") {
			return nil
		}
		relative, relErr := filepath.Rel(root, current)
		if relErr != nil {
			return relErr
		}
		relative = filepath.ToSlash(relative)
		record.FileNames = append(record.FileNames, relative)
		if !withContents {
			return nil
		}
		data, readErr := os.ReadFile(current)
		if readErr != nil {
			return readErr
		}
		record.Files[relative] = data
		return nil
	})
	if err != nil {
		return serviceskills.GlobalRecord{}, fmt.Errorf("read global skill %s: %w", name, err)
	}
	sort.Strings(record.FileNames)
	return record, nil
}

func (s *Store) readIndex() (indexFile, error) {
	index := indexFile{Skills: map[string]indexEntry{}}
	data, err := os.ReadFile(filepath.Join(s.root, indexFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return index, nil
		}
		return index, fmt.Errorf("read global skills index: %w", err)
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return indexFile{Skills: map[string]indexEntry{}}, fmt.Errorf("parse global skills index: %w", err)
	}
	if index.Skills == nil {
		index.Skills = map[string]indexEntry{}
	}
	return index, nil
}

func (s *Store) writeIndex(index indexFile) error {
	if index.Skills == nil {
		index.Skills = map[string]indexEntry{}
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal global skills index: %w", err)
	}
	tmp, err := os.CreateTemp(s.root, ".index-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp global skills index: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp global skills index: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp global skills index: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(s.root, indexFileName)); err != nil {
		return fmt.Errorf("replace global skills index: %w", err)
	}
	return nil
}

func sortedKeys(files map[string][]byte) []string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, path.Clean(key))
	}
	sort.Strings(keys)
	return keys
}
