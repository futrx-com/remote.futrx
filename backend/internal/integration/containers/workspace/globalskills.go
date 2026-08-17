package workspace

// Global-skill provisioning mirrors the host library
// (DATA_DIR/skills-global) into /workspace/.agents/skills-global and links
// each entry into the canonical /workspace/.agents/skills directory. The link
// is only created when the slot is free, which is what makes a project-local
// skill of the same name win: the global copy stays on disk but is never
// visible to the agent CLI.
//
// The whole library shares one hash marker, so the steady-state cost of a
// chat start is a single `lxc exec cat`.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

const (
	containerSkillsDir       = "/workspace/.agents/skills"
	containerGlobalSkillsDir = "/workspace/.agents/skills-global"
	containerGlobalMarker    = containerGlobalSkillsDir + "/.library.sha256"

	skillManifestName           = "SKILL.md"
	globalSkillsConvergeTimeout = 60 * time.Second
	globalSkillsQueryTimeout    = 10 * time.Second

	// maxGlobalSkillFileBytes mirrors the library's per-file limit so a
	// hand-edited DATA_DIR entry cannot push an unbounded file.
	maxGlobalSkillFileBytes = 256 * 1024
)

// Option customizes a Provisioner at composition time.
type Option func(*Provisioner)

// WithGlobalSkillLibrary points the provisioner at the host directory holding
// the platform-wide skills library. An empty path disables the sync.
func WithGlobalSkillLibrary(directory string) Option {
	return func(p *Provisioner) {
		p.globalSkillsDir = strings.TrimSpace(directory)
	}
}

type globalSkillFile struct {
	path    string
	content []byte
}

type globalSkill struct {
	name  string
	files []globalSkillFile
}

// EnsureGlobalSkills converges the container's copy of the global library.
// It publishes new or changed content, prunes entries the admin removed, and
// (re)creates the links that expose each global skill to the agent CLIs.
func (p *Provisioner) EnsureGlobalSkills(ctx context.Context, containerName string) error {
	if p.globalSkillsDir == "" {
		return nil
	}
	if !p.runner.Available() {
		return command.ErrUnavailable
	}

	library, err := loadGlobalSkillLibrary(p.globalSkillsDir)
	if err != nil {
		return err
	}
	want := globalLibraryHash(library)

	current, err := command.RunWithTimeout(
		ctx, p.runner, globalSkillsQueryTimeout,
		"exec", containerName, "--", "cat", containerGlobalMarker,
	)
	if err == nil && strings.TrimSpace(current) == want {
		return nil
	}

	cctx, cancel := context.WithTimeout(ctx, globalSkillsConvergeTimeout)
	defer cancel()

	if directories := globalSkillDirectories(library); len(directories) > 0 {
		args := append([]string{"exec", containerName, "--", "install", "-d", "-m", "755"}, directories...)
		if out, err := p.runner.Run(cctx, args...); err != nil {
			return fmt.Errorf("create global skill directories: %w; output: %s", err, out)
		}
	}

	for _, skill := range library {
		for _, file := range skill.files {
			destination := path.Join(containerGlobalSkillsDir, skill.name, file.path)
			if err := p.pushGlobalSkillFile(cctx, containerName, destination, file.content); err != nil {
				return err
			}
		}
	}

	script := globalSkillsSyncScript(library, want)
	if out, err := p.runner.Run(cctx, "exec", containerName, "--", "sh", "-c", script); err != nil {
		return fmt.Errorf("link global skills: %w; output: %s", err, out)
	}
	return nil
}

func (p *Provisioner) pushGlobalSkillFile(
	ctx context.Context,
	containerName string,
	destination string,
	content []byte,
) error {
	tmp, err := os.CreateTemp("", "futrx-global-skill-*")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write global skill file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close global skill file: %w", err)
	}
	if out, err := p.runner.Run(ctx, "file", "push", "--mode=644", tmp.Name(), containerName+destination); err != nil {
		return fmt.Errorf("push %s: %w; output: %s", destination, err, out)
	}
	return nil
}

// loadGlobalSkillLibrary reads the host library directory. Every direct child
// directory that carries a SKILL.md is one skill; reserved children (dotdirs,
// the `_index.json` policy file, staging leftovers) are skipped, matching the
// store's own view of the layout.
func loadGlobalSkillLibrary(root string) ([]globalSkill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read global skills library: %w", err)
	}

	library := make([]globalSkill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validGlobalSkillDirName(entry.Name()) {
			continue
		}
		directory := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(directory, skillManifestName)); err != nil {
			continue
		}
		files, err := readGlobalSkillFiles(directory)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			continue
		}
		library = append(library, globalSkill{name: entry.Name(), files: files})
	}
	sort.Slice(library, func(i, j int) bool { return library[i].name < library[j].name })
	return library, nil
}

func readGlobalSkillFiles(root string) ([]globalSkillFile, error) {
	var files []globalSkillFile
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
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxGlobalSkillFileBytes {
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		files = append(files, globalSkillFile{path: filepath.ToSlash(relative), content: data})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read global skill %s: %w", filepath.Base(root), err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, nil
}

// validGlobalSkillDirName keeps the container-side reader in step with
// serviceskills.ValidGlobalSkillName without importing the service layer.
func validGlobalSkillDirName(name string) bool {
	if name == "" || len(name) > 64 || name[0] == '.' || name[0] == '_' {
		return false
	}
	for index := 0; index < len(name); index++ {
		char := name[index]
		switch {
		case char >= 'a' && char <= 'z':
		case char >= '0' && char <= '9':
		case char == '-' || char == '_' || char == '.':
		default:
			return false
		}
	}
	return true
}

// globalLibraryHash fingerprints the whole library, including skill and file
// names, so a rename or deletion invalidates the container marker too.
func globalLibraryHash(library []globalSkill) string {
	digest := sha256.New()
	for _, skill := range library {
		fmt.Fprintf(digest, "%s\x00", skill.name)
		for _, file := range skill.files {
			fmt.Fprintf(digest, "%s\x00%d\x00", file.path, len(file.content))
			digest.Write(file.content)
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// globalSkillDirectories lists every container directory the push needs,
// parents first, so one `install -d` call can create them all.
func globalSkillDirectories(library []globalSkill) []string {
	seen := map[string]bool{containerGlobalSkillsDir: true}
	directories := []string{containerGlobalSkillsDir}
	for _, skill := range library {
		for _, file := range skill.files {
			directory := path.Join(containerGlobalSkillsDir, skill.name, path.Dir(file.path))
			for _, candidate := range ancestorDirectories(directory) {
				if seen[candidate] {
					continue
				}
				seen[candidate] = true
				directories = append(directories, candidate)
			}
		}
	}
	return directories
}

// ancestorDirectories walks up from directory to the library root and returns
// the chain parents-first so `install -d` never sees a child before its
// parent.
func ancestorDirectories(directory string) []string {
	var chain []string
	for current := directory; strings.HasPrefix(current, containerGlobalSkillsDir+"/"); current = path.Dir(current) {
		chain = append(chain, current)
	}
	for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
		chain[left], chain[right] = chain[right], chain[left]
	}
	return chain
}

// globalSkillsSyncScript prunes removed entries and links the current ones.
// The link is skipped whenever anything already occupies the canonical slot,
// which is how a project-local skill shadows a global one.
func globalSkillsSyncScript(library []globalSkill, marker string) string {
	var script strings.Builder
	script.WriteString(`set -eu
canonical=` + containerSkillsDir + `
global=` + containerGlobalSkillsDir + `
mkdir -p "$canonical" "$global"
chmod 755 "$canonical" "$global"

# Drop links that point at a global skill which no longer exists.
for link in "$canonical"/*; do
  if [ ! -L "$link" ]; then continue; fi
  target=$(readlink "$link")
  case "$target" in
    ../skills-global/*) ;;
    *) continue ;;
  esac
  name=$(basename "$link")
  case "$name" in
`)
	script.WriteString("    " + skillNameCasePattern(library) + ") ;;\n")
	script.WriteString(`    *) rm -f "$link" ;;
  esac
done

# Drop published content for skills the admin removed.
for entry in "$global"/*; do
  if [ ! -d "$entry" ]; then continue; fi
  name=$(basename "$entry")
  case "$name" in
`)
	script.WriteString("    " + skillNameCasePattern(library) + ") ;;\n")
	script.WriteString(`    *) rm -rf "$entry" ;;
  esac
done

link_global_skill() {
  name="$1"
  link="$canonical/$name"
  target="../skills-global/$name"
  if [ -L "$link" ]; then
    if [ "$(readlink "$link")" != "$target" ]; then
      rm -f "$link"
      ln -s "$target" "$link"
    fi
  elif [ ! -e "$link" ]; then
    ln -s "$target" "$link"
  fi
}
`)
	for _, skill := range library {
		fmt.Fprintf(&script, "link_global_skill %s\n", shellQuote(skill.name))
	}
	fmt.Fprintf(&script, "printf '%%s' %s > %s\n", shellQuote(marker), shellQuote(containerGlobalMarker))
	return script.String()
}

// skillNameCasePattern renders the keep-list of a `case` statement. An empty
// library yields a pattern basename never produces, so everything is pruned.
func skillNameCasePattern(library []globalSkill) string {
	if len(library) == 0 {
		return "''"
	}
	patterns := make([]string, 0, len(library))
	for _, skill := range library {
		patterns = append(patterns, shellQuote(skill.name))
	}
	return strings.Join(patterns, "|")
}
