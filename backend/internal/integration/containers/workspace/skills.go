package workspace

// Project skills live in /workspace/.agents/skills. Agent-specific workspace
// homes declared by profiles are compatibility links to that source of truth.

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

const ensureWorkspaceSymlinksTimeout = 10 * time.Second

const canonicalWorkspaceSkillsDir = "/workspace/.agents/skills"

// EnsureSkillLinks creates the canonical .agents skills directory, migrates
// legacy skill children when possible, publishes the code-owned default
// skills, and points each configured compatibility path at .agents/skills.
// Cheap and idempotent.
func (p *Provisioner) EnsureSkillLinks(ctx context.Context, containerName string) error {
	if !p.runner.Available() {
		return command.ErrUnavailable
	}
	profiles := p.profiles.Snapshot()
	if err := p.ensureSkillTopology(ctx, containerName, profiles); err != nil {
		return err
	}
	if err := p.ensureDefaultSkills(ctx, containerName); err != nil {
		return err
	}
	// Home-level compatibility directories mirror canonical children rather
	// than linking the directory itself, so refresh them after new defaults
	// have been published.
	return p.ensureSkillTopology(ctx, containerName, profiles)
}

func (p *Provisioner) ensureSkillTopology(ctx context.Context, containerName string, profiles []provisioning.Profile) error {
	script := workspaceSkillLinksScript(profiles)
	if _, err := command.RunWithTimeout(ctx, p.runner, ensureWorkspaceSymlinksTimeout, "exec", containerName, "--", "sh", "-c", script); err != nil {
		return fmt.Errorf("ensure workspace skill topology: %w", err)
	}
	return nil
}

func (p *Provisioner) ensureDefaultSkills(ctx context.Context, containerName string) error {
	if p.publisher == nil {
		return errors.New("default skill publisher not configured")
	}
	skills := provisioning.DefaultSkills()
	if len(skills) == 0 {
		return nil
	}

	directories := map[string]struct{}{}
	for _, skill := range skills {
		for _, asset := range skill.Assets {
			destination := path.Join(canonicalWorkspaceSkillsDir, skill.Command, asset.Path)
			directories[path.Dir(destination)] = struct{}{}
		}
	}
	orderedDirectories := make([]string, 0, len(directories))
	for directory := range directories {
		orderedDirectories = append(orderedDirectories, directory)
	}
	sort.Strings(orderedDirectories)

	dctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := []string{"exec", containerName, "--", "install", "-d", "-m", "755"}
	args = append(args, orderedDirectories...)
	if out, err := p.runner.Run(dctx, args...); err != nil {
		return fmt.Errorf("create default skill directories: %w; output: %s", err, out)
	}

	for _, skill := range skills {
		for _, asset := range skill.Assets {
			destination := path.Join(canonicalWorkspaceSkillsDir, skill.Command, asset.Path)
			hashPath := path.Join(path.Dir(destination), "."+path.Base(destination)+".remote.sha256")
			if err := p.publisher.PushVerified(ctx, containerName, asset.Content, hashPath, "644", destination); err != nil {
				return fmt.Errorf("publish default skill %s/%s: %w", skill.Command, asset.Path, err)
			}
		}
	}
	return nil
}

func workspaceSkillLinksScript(profiles []provisioning.Profile) string {
	workspaceHomes := make([]string, 0, len(profiles))
	homeSkillDirs := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if profile.WorkspaceSkills == nil {
			continue
		}
		workspaceHomes = appendUnique(workspaceHomes, profile.WorkspaceSkills.WorkspaceHome)
		if profile.WorkspaceSkills.HomeSkillsDir != "" {
			homeSkillDirs = appendUnique(homeSkillDirs, profile.WorkspaceSkills.HomeSkillsDir)
		}
	}

	var script strings.Builder
	script.WriteString(`set -eu
canonical=/workspace/.agents/skills
mkdir -p /workspace/.agents "$canonical"`)
	for _, home := range workspaceHomes {
		script.WriteByte(' ')
		script.WriteString(shellQuote(home))
	}
	script.WriteString(`
chmod 755 /workspace/.agents "$canonical"`)
	for _, home := range workspaceHomes {
		script.WriteByte(' ')
		script.WriteString(shellQuote(home))
	}
	script.WriteString(`

migrate_skills_dir() {
  src="$1"
  [ -e "$src" ] || return 0
  [ ! -L "$src" ] || return 0
  [ -d "$src" ] || return 0

  for entry in "$src"/* "$src"/.[!.]* "$src"/..?*; do
    [ -e "$entry" ] || continue
    name=$(basename "$entry")
    [ "$name" != "." ] && [ "$name" != ".." ] || continue
    target="$canonical/$name"
    if [ ! -e "$target" ] && [ ! -L "$target" ]; then
      mv "$entry" "$target"
    fi
  done
  rmdir "$src" 2>/dev/null || true
}

link_skills_dir() {
  base="$1"
  target="$2"
  link="$base/skills"
  if [ -L "$link" ]; then
    current=$(readlink "$link")
    if [ "$current" != "$target" ]; then
      rm "$link"
      ln -s "$target" "$link"
    fi
  elif [ ! -e "$link" ]; then
    ln -s "$target" "$link"
  fi
}

mirror_home_skills() {
  home_skills="$1"
  [ -d "$(dirname "$home_skills")" ] || return 0
  mkdir -p "$home_skills"
  for entry in "$home_skills"/* ; do
    [ -e "$entry" ] && continue            # resolves fine (real dir or live link) → keep
    [ -L "$entry" ] && rm -f "$entry"      # dangling symlink → prune
  done
  if [ -d "$canonical" ]; then
    for d in "$canonical"/*/ ; do
      [ -d "$d" ] || continue
      name=$(basename "$d")
      [ "$name" = ".system" ] && continue
      ln -sfn "$canonical/$name" "$home_skills/$name"
    done
  fi
}
`)
	for _, home := range workspaceHomes {
		fmt.Fprintf(&script, "migrate_skills_dir %s\n", shellQuote(path.Join(home, "skills")))
	}
	for _, home := range workspaceHomes {
		relativeTarget, err := filepath.Rel(home, "/workspace/.agents/skills")
		if err != nil {
			relativeTarget = "/workspace/.agents/skills"
		}
		relativeTarget = filepath.ToSlash(relativeTarget)
		fmt.Fprintf(&script, "link_skills_dir %s %s\n", shellQuote(home), shellQuote(relativeTarget))
	}
	for _, homeSkills := range homeSkillDirs {
		fmt.Fprintf(&script, "mirror_home_skills %s\n", shellQuote(homeSkills))
	}
	return script.String()
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
