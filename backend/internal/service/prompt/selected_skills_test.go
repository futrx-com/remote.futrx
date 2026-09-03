package prompt

import (
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type testAgentPolicy map[string]agentmodule.Descriptor

func (p testAgentPolicy) Descriptor(provider string) (agentmodule.Descriptor, bool) {
	descriptor, ok := p[provider]
	return descriptor, ok
}

func (p testAgentPolicy) SupportsScope(provider string, scope agentmodule.ExecutionScope) bool {
	descriptor, ok := p[provider]
	if !ok {
		return false
	}
	if len(descriptor.ExecutionScopes) == 0 {
		return true
	}
	for _, configured := range descriptor.ExecutionScopes {
		if configured == scope {
			return true
		}
	}
	return false
}

func codexTestAgentPolicy() testAgentPolicy {
	return testAgentPolicy{"codex": {
		ID:    agent.ProviderCodex,
		Label: "Codex",
		Features: agentmodule.Features{
			Skills:         agentmodule.SkillsDollarMention,
			BrowserTools:   true,
			ScheduledTools: true,
		},
	}}
}

func TestPromptWithSelectedSkillsPrefixesClaudeSlashCommands(t *testing.T) {
	got := promptWithSelectedSkills(agentmodule.SkillsSlashCommand, "Claude", agent.ProviderClaude, []servicechat.SkillRef{
		{Name: "Frontend Design", Command: "frontend-design", Provider: servicechat.ProviderClaude},
	}, true, "build the UI")

	want := "/frontend-design\n\nbuild the UI"
	if got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func TestPromptWithSelectedSkillsPrefixesCodexDollarTriggers(t *testing.T) {
	got := promptWithSelectedSkills(agentmodule.SkillsDollarMention, "Codex", agent.ProviderCodex, []servicechat.SkillRef{
		{Name: "Frontend Design", Command: "frontend-design", Provider: servicechat.ProviderCodex},
		{Name: "Review", Command: "review", Provider: servicechat.ProviderCodex},
	}, true, "build the UI")

	if !strings.HasPrefix(got, "Use these Codex skills for this request: $frontend-design $review\n\n") {
		t.Fatalf("missing codex skill prefix: %q", got)
	}
	if !strings.HasSuffix(got, "build the UI") {
		t.Fatalf("missing original prompt: %q", got)
	}
}

func TestPromptWithSelectedSkillsFiltersOtherProviders(t *testing.T) {
	got := promptWithSelectedSkills(agentmodule.SkillsDollarMention, "Codex", agent.ProviderCodex, []servicechat.SkillRef{
		{Name: "Claude Only", Command: "claude-only", Provider: servicechat.ProviderClaude},
	}, true, "ship it")

	if got != "ship it" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestPromptWithSelectedSkillsLoadsScheduledTasksForKimi(t *testing.T) {
	got := promptWithSelectedSkills(agentmodule.SkillsInstructions, "Kimi", agent.ProviderKimi, []servicechat.SkillRef{{
		Name:     "Scheduled Tasks",
		Command:  "scheduled-tasks",
		Provider: servicechat.ProviderKimi,
	}}, true, "watch the deploy")

	if !strings.Contains(got, "/workspace/.agents/skills/scheduled-tasks/SKILL.md") {
		t.Fatalf("Kimi prompt missing scheduled-task skill path: %q", got)
	}
	if !strings.HasSuffix(got, "watch the deploy") {
		t.Fatalf("Kimi prompt missing user request: %q", got)
	}
}

func TestSkillTriggerNameFallsBackToSingleToken(t *testing.T) {
	got := promptWithSelectedSkills(agentmodule.SkillsDollarMention, "Codex", agent.ProviderCodex, []servicechat.SkillRef{
		{Name: "Frontend Design", Provider: servicechat.ProviderCodex},
	}, true, "ship it")

	if !strings.Contains(got, "$Frontend-Design") {
		t.Fatalf("prompt = %q", got)
	}
}

func TestPromptWithSelectedSkillsSupportsFutureInstructionAgent(t *testing.T) {
	got := promptWithSelectedSkills(
		agentmodule.SkillsInstructions,
		"Future Agent",
		"future-agent",
		[]servicechat.SkillRef{{Name: "Review", Command: "review", Provider: "future-agent"}},
		false,
		"ship it",
	)
	if !strings.Contains(got, "/root/.agents/skills/review/SKILL.md") {
		t.Fatalf("future-agent prompt missing skill instructions: %q", got)
	}
}

func TestHasBrowserSkill(t *testing.T) {
	if !hasBrowserSkill([]servicechat.SkillRef{{Name: "Browser", Command: "browser"}}) {
		t.Fatal("expected browser skill command to enable browser tools")
	}
	if hasBrowserSkill([]servicechat.SkillRef{{Name: "Review", Command: "review"}}) {
		t.Fatal("non-browser skill should not enable browser tools")
	}
}
