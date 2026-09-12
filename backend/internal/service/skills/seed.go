package skills

// Built-in global skills ship inside the binary so a fresh install has a
// useful library on first boot. They are installed only while the library is
// empty: once an operator has authored, edited, or deleted anything, the
// platform never writes over that decision.

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed assets/global
var builtinGlobalSkills embed.FS

const builtinGlobalSkillsRoot = "assets/global"

// SeedBuiltins installs the embedded global skills when the library holds no
// entries. It reports how many skills it installed; zero means the library
// was already populated and was left untouched.
func (s *GlobalService) SeedBuiltins(ctx context.Context) (int, error) {
	if s == nil || s.repo == nil {
		return 0, ErrGlobalLibraryUnavailable
	}
	existing, err := s.repo.List(ctx)
	if err != nil {
		return 0, err
	}
	if len(existing) > 0 {
		return 0, nil
	}

	seeds, err := BuiltinGlobalSkills()
	if err != nil {
		return 0, err
	}
	installed := 0
	for _, seed := range seeds {
		files, err := NormalizeGlobalSkillFiles(seed.Files)
		if err != nil {
			return installed, fmt.Errorf("built-in skill %s: %w", seed.Name, err)
		}
		if _, err := s.repo.Save(ctx, GlobalRecord{Name: seed.Name, Files: files}); err != nil {
			return installed, err
		}
		installed++
	}
	return installed, nil
}

// BuiltinSeed is one embedded global skill.
type BuiltinSeed struct {
	Name  string
	Files map[string]string
}

// BuiltinGlobalSkills returns the embedded seed content in a stable order.
func BuiltinGlobalSkills() ([]BuiltinSeed, error) {
	entries, err := builtinGlobalSkills.ReadDir(builtinGlobalSkillsRoot)
	if err != nil {
		return nil, fmt.Errorf("read built-in global skills: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	seeds := make([]BuiltinSeed, 0, len(names))
	for _, name := range names {
		root := path.Join(builtinGlobalSkillsRoot, name)
		files := map[string]string{}
		walkErr := fs.WalkDir(builtinGlobalSkills, root, func(current string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			data, readErr := builtinGlobalSkills.ReadFile(current)
			if readErr != nil {
				return readErr
			}
			files[strings.TrimPrefix(current, root+"/")] = string(data)
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("read built-in global skill %s: %w", name, walkErr)
		}
		seeds = append(seeds, BuiltinSeed{Name: name, Files: files})
	}
	return seeds, nil
}
