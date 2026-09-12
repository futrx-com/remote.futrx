package workspace

// Project skills live in /workspace/.agents/skills. Agent-specific workspace
// homes declared by profiles are compatibility links to that source of truth.

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

const ensureWorkspaceSymlinksTimeout = 10 * time.Second

// EnsureSkillLinks creates the canonical .agents skills directory, migrates
// legacy skill children when possible, and points each configured
// compatibility path at .agents/skills. Cheap and idempotent.
//
// The global skills library converges first so the per-provider home-directory
// mirroring below picks up global entries in the same pass. A failing global
// sync must not cost a project its own skill links, so its error is reported
// only after the local links are in place.
func (p *Provisioner) EnsureSkillLinks(ctx context.Context, containerName string) error {
	if !p.runner.Available() {
		return command.ErrUnavailable
	}
	globalErr := p.EnsureGlobalSkills(ctx, containerName)
	script := workspaceSkillLinksScript(p.profiles.Snapshot())
	if _, err := command.RunWithTimeout(ctx, p.runner, ensureWorkspaceSymlinksTimeout, "exec", containerName, "--", "sh", "-c", script); err != nil {
		return err
	}
	return globalErr
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
