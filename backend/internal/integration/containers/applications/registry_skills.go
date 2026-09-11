package applications

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
)

const (
	// skillsDir is where an image ships agent skills. Its presence opts the
	// image in; nothing in image.json declares it, the same way ui/ works.
	skillsDir = "skills"
	// skillFileName is what makes a directory a skill rather than a folder of
	// loose files.
	skillFileName = "SKILL.md"
	// skillHashFile marks published content so an unchanged skill is not
	// re-pushed on every install.
	skillHashFile = ".skill.sha256"
)

// skillNamePattern keeps a skill to one path component. These names become
// directories inside a project's workspace, so nothing else is accepted.
var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// loadImageSkills lists the skills an image ships, in a stable order. A missing
// skills/ directory is not an error: most images ship none.
func loadImageSkills(fsys fs.FS, root string) ([]string, error) {
	entries, err := fs.ReadDir(fsys, root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !skillNamePattern.MatchString(entry.Name()) {
			return nil, fmt.Errorf("skill %q: name must be lowercase words joined by hyphens", entry.Name())
		}
		if _, err = fs.Stat(fsys, path.Join(root, entry.Name(), skillFileName)); err != nil {
			return nil, fmt.Errorf("skill %q: missing %s", entry.Name(), skillFileName)
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		return nil, nil
	}
	sort.Strings(names)
	return names, nil
}

// Skill returns the SKILL.md an image ships under the given name. Only names
// the registry already validated are readable, so a caller cannot reach
// outside the image's own skills directory.
func (r *Registry) Skill(id, name string) ([]byte, bool) {
	img, source, ok := r.imageSource(id)
	if !ok || source == nil {
		return nil, false
	}
	for _, shipped := range img.Skills {
		if shipped != name {
			continue
		}
		body, err := fs.ReadFile(source, path.Join(catalogRoot, id, skillsDir, name, skillFileName))
		if err != nil {
			return nil, false
		}
		return body, true
	}
	return nil, false
}
