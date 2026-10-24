package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

func instructionTestProfiles() []provisioning.Profile {
	return []provisioning.Profile{
		{ID: "claude", Instructions: &provisioning.InstructionTarget{Path: "/root/.claude/CLAUDE.md"}},
		{ID: "codex", Instructions: &provisioning.InstructionTarget{Path: "/root/.codex/AGENTS.md"}},
		{ID: "other"},
	}
}

func TestAgentInstructionsComposeWithoutMutatingProfiles(t *testing.T) {
	file := filepath.Join(t.TempDir(), "instructions.json")
	if err := os.WriteFile(file, []byte(`{"global":"Global policy", "providers":{"claude":"Claude policy"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	profiles := instructionTestProfiles()
	configured, shared, err := AgentInstructionProfiles(file, "example.test", profiles)
	if err != nil {
		t.Fatal(err)
	}
	claude := string(configured[0].Instructions.Content)
	codex := string(configured[1].Instructions.Content)
	if !strings.Contains(claude, "example.test") || !strings.Contains(claude, "Global policy") || !strings.HasSuffix(claude, "Claude policy\n") {
		t.Fatal("missing composed instructions")
	}
	if strings.Index(claude, "Global policy") > strings.Index(claude, "Claude policy") {
		t.Fatal("wrong composition order")
	}
	if strings.Contains(codex, "Claude policy") || !bytes.Equal(shared, configured[1].Instructions.Content) {
		t.Fatal("provider content leaked")
	}
	if profiles[0].Instructions.Content != nil || configured[2].Instructions != nil {
		t.Fatal("mutated source profiles or invented target")
	}
	clone := configured[0].Clone()
	clone.Instructions.Content[0] = '!'
	if configured[0].Instructions.Content[0] == '!' {
		t.Fatal("instruction content aliases cloned profile")
	}
}

func TestAgentInstructionsDefaultAndEnvironment(t *testing.T) {
	t.Setenv("AGENT_INSTRUCTIONS_FILE", "/operator/instructions.json")
	if Load().Agent.InstructionsFile != "/operator/instructions.json" {
		t.Fatal("environment not loaded")
	}
	configured, shared, err := AgentInstructionProfiles("", "example.test", instructionTestProfiles())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(shared, provisioning.InstructionsTemplate("example.test")) || !bytes.Equal(shared, configured[0].Instructions.Content) {
		t.Fatal("default template changed")
	}
}

func TestAgentInstructionsRejectInvalidConfiguration(t *testing.T) {
	for name, content := range map[string]string{
		"invalid": "{", "null": "null", "array": "[]", "empty": "", "trailing": "{} {}",
		"unknown field": `{"globla":"policy"}`, "unknown provider": `{"providers":{"typo":"policy"}}`,
		"unsupported target": `{"providers":{"other":"policy"}}`,
		"oversized":          strings.Repeat(" ", maxAgentInstructionsBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "instructions.json")
			if err := os.WriteFile(file, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := AgentInstructionProfiles(file, "example.test", instructionTestProfiles()); err == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
	if _, _, err := AgentInstructionProfiles(filepath.Join(t.TempDir(), "missing"), "example.test", instructionTestProfiles()); err == nil {
		t.Fatal("silently ignored missing configuration")
	}
}
