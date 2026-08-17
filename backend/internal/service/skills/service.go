package skills

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SkillFileName is the manifest every skill directory must contain.
const SkillFileName = "SKILL.md"

type Service struct {
	agentsHome string
	claudeHome string
	codexHome  string
}

type rootSpec struct {
	path   string
	source string
}

func New() *Service {
	return NewWithSkillHomes(defaultAgentsHome(), defaultClaudeHome(), defaultCodexHome())
}

func NewWithHomes(claudeHome, codexHome string) *Service {
	return NewWithSkillHomes("", claudeHome, codexHome)
}

func NewWithSkillHomes(agentsHome, claudeHome, codexHome string) *Service {
	return &Service{
		agentsHome: agentsHome,
		claudeHome: claudeHome,
		codexHome:  codexHome,
	}
}

func (s *Service) List(ctx context.Context, provider Provider, projectWorkspace string) ([]Skill, error) {
	switch provider {
	case ProviderClaude, ProviderCodex, ProviderKimi, ProviderAntigravity:
	default:
		return nil, ErrInvalidProvider
	}

	var skills []Skill
	for _, root := range s.roots(provider) {
		if err := collectSkills(ctx, provider, root, &skills); err != nil {
			return nil, err
		}
	}
	// .agents/skills is the project source of truth. The provider-specific
	// paths are legacy compatibility fallbacks and are deduped below.
	if projectWorkspace != "" {
		for _, root := range projectRoots(projectWorkspace) {
			if err := collectSkills(ctx, provider, root, &skills); err != nil {
				return nil, err
			}
		}
		if !hasSkillCommand(skills, "scheduled-tasks") {
			skills = append(skills, Skill{
				Name:        "Scheduled Tasks",
				Command:     "scheduled-tasks",
				Description: "Create and manage persistent one-time or recurring tasks that return to this chat.",
				Provider:    provider,
				Source:      "remote",
			})
		}
	}
	if skills == nil {
		skills = []Skill{}
	}

	skills = dedupeSkills(skills)
	SortSkills(skills)
	return skills, nil
}

func hasSkillCommand(skills []Skill, command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	for _, skill := range skills {
		if strings.ToLower(strings.TrimSpace(skill.Command)) == command {
			return true
		}
	}
	return false
}

func (s *Service) roots(provider Provider) []rootSpec {
	// We deliberately do NOT walk plugins/cache or the .system subtree —
	// those hold CLI-bundled skills the user didn't author and asked us to
	// keep out of the picker (see collectSkills for the dotdir skip).
	var roots []rootSpec
	if s.agentsHome != "" {
		roots = append(roots, rootSpec{path: filepath.Join(s.agentsHome, "skills"), source: "user"})
	}
	switch provider {
	case ProviderClaude:
		return append(roots, rootSpec{path: filepath.Join(s.claudeHome, "skills"), source: "user"})
	case ProviderCodex:
		return append(roots, rootSpec{path: filepath.Join(s.codexHome, "skills"), source: "user"})
	case ProviderKimi, ProviderAntigravity:
		return roots
	default:
		return nil
	}
}

func projectRoots(projectWorkspace string) []rootSpec {
	return []rootSpec{
		{path: filepath.Join(projectWorkspace, ".agents", "skills"), source: "project"},
		{path: filepath.Join(projectWorkspace, ".claude", "skills"), source: "project"},
		{path: filepath.Join(projectWorkspace, ".codex", "skills"), source: "project"},
	}
}

func dedupeSkills(skills []Skill) []Skill {
	if len(skills) <= 1 {
		return skills
	}
	seen := map[string]bool{}
	out := make([]Skill, 0, len(skills))
	for _, skill := range skills {
		command := strings.TrimSpace(skill.Command)
		if command == "" {
			command = strings.TrimSpace(skill.Name)
		}
		key := strings.ToLower(string(skill.Provider)) + "\x00" + strings.ToLower(skill.Source) + "\x00" + strings.ToLower(command)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, skill)
	}
	return out
}

func collectSkills(ctx context.Context, provider Provider, root rootSpec, out *[]Skill) error {
	if root.path == "" {
		return nil
	}
	if _, err := os.Stat(root.path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat skills root %s: %w", root.path, err)
	}

	return filepath.WalkDir(root.path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Skip hidden directories below the root (e.g. .system, .cache) —
		// those are CLI-bundled, not user-authored. The root itself starts
		// with a dot in project workspaces, so we only filter children, not
		// the root.
		if d.IsDir() && path != root.path && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() || d.Name() != SkillFileName {
			return nil
		}

		skill, err := readSkillFile(path)
		if err != nil {
			return nil
		}
		skill.Command = strings.TrimSpace(filepath.Base(filepath.Dir(path)))
		if skill.Name == "" {
			skill.Name = skill.Command
		}
		skill.Name = strings.TrimSpace(skill.Name)
		if skill.Name == "" {
			return nil
		}
		skill.Provider = provider
		skill.Source = sourceForPath(root, path)
		*out = append(*out, skill)
		return nil
	})
}

func readSkillFile(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	if len(data) > 64*1024 {
		data = data[:64*1024]
	}
	return parseSkillMetadata(data), nil
}

func parseSkillMetadata(data []byte) Skill {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 4096), 64*1024)

	var skill Skill
	inFrontMatter := false
	sawFrontMatter := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			if !sawFrontMatter {
				sawFrontMatter = true
				inFrontMatter = true
				continue
			}
			break
		}
		if !inFrontMatter {
			if strings.HasPrefix(line, "# ") && skill.Name == "" {
				skill.Name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			}
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = cleanYAMLScalar(value)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "name":
			skill.Name = value
		case "description":
			skill.Description = value
		}
	}
	return skill
}

func cleanYAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return strings.TrimSpace(value)
}

func sourceForPath(root rootSpec, path string) string {
	rel, err := filepath.Rel(root.path, path)
	if err != nil {
		return root.source
	}
	if strings.HasPrefix(rel, ".system"+string(filepath.Separator)) {
		return "system"
	}
	return root.source
}

func defaultAgentsHome() string {
	if value := os.Getenv("AGENTS_HOME"); value != "" {
		return value
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".agents")
	}
	return "/root/.agents"
}

func defaultClaudeHome() string {
	if value := os.Getenv("CLAUDE_CONFIG_DIR"); value != "" {
		return value
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".claude")
	}
	return "/root/.claude"
}

func defaultCodexHome() string {
	if value := os.Getenv("CODEX_HOME"); value != "" {
		return value
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".codex")
	}
	return "/root/.codex"
}
