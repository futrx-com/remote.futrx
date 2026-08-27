package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

type skillTestProviderCatalog map[string]agentmodule.Descriptor

func (c skillTestProviderCatalog) HasProvider(provider string) bool {
	_, ok := c[provider]
	return ok
}

func (c skillTestProviderCatalog) Descriptor(provider string) (agentmodule.Descriptor, bool) {
	descriptor, ok := c[provider]
	return descriptor, ok
}

func (c skillTestProviderCatalog) SupportsScope(provider string, scope agentmodule.ExecutionScope) bool {
	descriptor, ok := c[provider]
	if !ok {
		return false
	}
	for _, configured := range descriptor.ExecutionScopes {
		if configured == scope {
			return true
		}
	}
	return false
}

func (c skillTestProviderCatalog) LegacySkillRoots(string) []string { return nil }

func (c skillTestProviderCatalog) WorkspaceSkillHome(provider string) string {
	if provider == "future-agent" {
		return "/workspace/.future"
	}
	return ""
}

type defaultSkillTestProviderCatalog struct {
	skillTestProviderCatalog
	provider Provider
}

func (c defaultSkillTestProviderCatalog) DefaultProvider(agentmodule.ExecutionScope) Provider {
	return c.provider
}

func TestDefaultProviderUsesModuleCatalog(t *testing.T) {
	catalog := defaultSkillTestProviderCatalog{
		skillTestProviderCatalog: skillTestProviderCatalog{"future-agent": {
			ID: "future-agent", ExecutionScopes: []agentmodule.ExecutionScope{agentmodule.ScopeHost},
		}},
		provider: "future-agent",
	}
	service := NewWithSkillHomes(t.TempDir(), t.TempDir(), t.TempDir(), WithProviderCatalog(catalog))
	if got := service.DefaultProvider(agentmodule.ScopeHost); got != "future-agent" {
		t.Fatalf("default provider = %q, want future-agent", got)
	}
}

func TestListSkillsFiltersBundled(t *testing.T) {
	// CLI-bundled skills live under .system and plugins/cache; the
	// picker should ignore both and only surface user-authored skills.
	agentsHome := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, "skills", ".system", "openai-docs", "SKILL.md"), `---
name: "openai-docs"
description: "Use official OpenAI docs."
---
`)
	writeSkill(t, filepath.Join(agentsHome, "skills", "custom", "SKILL.md"), `# Custom Skill`)
	writeSkill(t, filepath.Join(home, "plugins", "cache", "plugin-a", "skills", "github", "SKILL.md"), `---
name: github
description: Triage GitHub work.
---
`)

	service := NewWithSkillHomes(agentsHome, t.TempDir(), home)
	got, err := service.List(context.Background(), ProviderCodex, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected only the user-authored skill, got %#v", got)
	}
	if got[0].Name != "Custom Skill" || got[0].Command != "custom" || got[0].Source != "user" {
		t.Fatalf("unexpected skill metadata: %#v", got[0])
	}
}

func TestListSkillsUsesAgentsAsProjectSourceOfTruth(t *testing.T) {
	workspace := t.TempDir()
	writeSkill(t, filepath.Join(workspace, ".agents", "skills", "custom", "SKILL.md"), `# Custom Skill`)
	writeSkill(t, filepath.Join(workspace, ".claude", "skills", "custom", "SKILL.md"), `# Legacy Duplicate`)
	writeSkill(t, filepath.Join(workspace, ".claude", "skills", "legacy", "SKILL.md"), `# Legacy Skill`)

	service := NewWithSkillHomes(filepath.Join(t.TempDir(), "missing-agents"), filepath.Join(t.TempDir(), "missing-claude"), filepath.Join(t.TempDir(), "missing-codex"))
	got, err := service.List(context.Background(), ProviderClaude, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected canonical, legacy fallback, and scheduled task skills, got %#v", got)
	}
	if got[0].Command != "custom" || got[0].Name != "Custom Skill" {
		t.Fatalf("canonical skill should win duplicates, got %#v", got[0])
	}
	if got[1].Command != "legacy" {
		t.Fatalf("expected legacy fallback skill, got %#v", got[1])
	}
	if got[2].Command != "scheduled-tasks" || got[2].Source != "remote" {
		t.Fatalf("expected the built-in scheduled task skill, got %#v", got[2])
	}
}

func TestListSkillsDoesNotDuplicateProjectScheduledTaskSkill(t *testing.T) {
	workspace := t.TempDir()
	writeSkill(t, filepath.Join(workspace, ".agents", "skills", "scheduled-tasks", "SKILL.md"), `# Workspace Scheduled Tasks`)

	service := NewWithSkillHomes("", t.TempDir(), t.TempDir())
	got, err := service.List(context.Background(), ProviderCodex, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected the project skill to suppress the built-in fallback, got %#v", got)
	}
	if got[0].Name != "Workspace Scheduled Tasks" || got[0].Source != "project" {
		t.Fatalf("expected the project-defined skill, got %#v", got[0])
	}
}

func TestListSkillsDedupesUserCompatibilityPaths(t *testing.T) {
	agentsHome := t.TempDir()
	codexHome := t.TempDir()
	writeSkill(t, filepath.Join(agentsHome, "skills", "custom", "SKILL.md"), `# Canonical Skill`)
	writeSkill(t, filepath.Join(codexHome, "skills", "custom", "SKILL.md"), `# Legacy Duplicate`)

	service := NewWithSkillHomes(agentsHome, t.TempDir(), codexHome)
	got, err := service.List(context.Background(), ProviderCodex, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected duplicate user skill to be collapsed, got %#v", got)
	}
	if got[0].Name != "Canonical Skill" || got[0].Command != "custom" {
		t.Fatalf("canonical user skill should win duplicate, got %#v", got[0])
	}
}

func TestListMissingRootsReturnsEmptyList(t *testing.T) {
	service := NewWithSkillHomes(filepath.Join(t.TempDir(), "missing-agents"), filepath.Join(t.TempDir(), "missing-claude"), filepath.Join(t.TempDir(), "missing-codex"))
	got, err := service.List(context.Background(), ProviderClaude, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty skill list, got %#v", got)
	}
}

func TestListRejectsInvalidProvider(t *testing.T) {
	service := NewWithHomes(t.TempDir(), t.TempDir())
	_, err := service.List(context.Background(), Provider("bad provider"), "")
	if !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("expected invalid provider error, got %v", err)
	}
}

func TestListSupportsFutureProviderFromCanonicalSkillsRoot(t *testing.T) {
	agentsHome := t.TempDir()
	writeSkill(t, filepath.Join(agentsHome, "skills", "custom", "SKILL.md"), `# Custom Skill`)
	service := NewWithSkillHomes(agentsHome, t.TempDir(), t.TempDir())

	got, err := service.List(context.Background(), Provider("future-agent"), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Provider != "future-agent" || got[0].Command != "custom" {
		t.Fatalf("future provider skills = %#v", got)
	}
}

func TestListUsesFutureProviderSkillAndScopeDeclaration(t *testing.T) {
	workspace := t.TempDir()
	writeSkill(t, filepath.Join(workspace, ".agents", "skills", "future", "SKILL.md"), `# Future Skill`)
	writeSkill(t, filepath.Join(workspace, ".future", "skills", "compatible", "SKILL.md"), `# Compatible Skill`)
	writeSkill(t, filepath.Join(workspace, ".claude", "skills", "claude-only", "SKILL.md"), `# Claude Only`)
	catalog := skillTestProviderCatalog{"future-agent": {
		ID:              "future-agent",
		ExecutionScopes: []agentmodule.ExecutionScope{agentmodule.ScopeProject},
		Features: agentmodule.Features{
			Skills: agentmodule.SkillsInstructions,
		},
	}}
	service := NewWithSkillHomes(t.TempDir(), t.TempDir(), t.TempDir(), WithProviderCatalog(catalog))

	got, err := service.List(context.Background(), "future-agent", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Command != "compatible" || got[1].Command != "future" {
		t.Fatalf("future project skills = %#v", got)
	}
	if _, err := service.List(context.Background(), "future-agent", ""); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("future host skills error = %v, want ErrInvalidProvider", err)
	}
}

func TestListReturnsEmptyWhenModuleDisablesSkills(t *testing.T) {
	agentsHome := t.TempDir()
	writeSkill(t, filepath.Join(agentsHome, "skills", "custom", "SKILL.md"), `# Custom`)
	catalog := skillTestProviderCatalog{"no-skills": {
		ID:              "no-skills",
		ExecutionScopes: []agentmodule.ExecutionScope{agentmodule.ScopeHost},
		Features:        agentmodule.Features{Skills: agentmodule.SkillsNone},
	}}
	service := NewWithSkillHomes(agentsHome, t.TempDir(), t.TempDir(), WithProviderCatalog(catalog))
	got, err := service.List(context.Background(), "no-skills", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("skills-disabled provider returned %#v", got)
	}
}

func writeSkill(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
