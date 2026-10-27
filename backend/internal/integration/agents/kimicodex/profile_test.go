package kimicodex

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

func TestProfileUsesIsolatedCodexHome(t *testing.T) {
	profile := Profile()
	if profile.ID != "kimi-codex" || profile.CLI.Binary != "codex" ||
		profile.CLI.PackageName != "@openai/codex" ||
		profile.CLI.Version != provisioning.MustCLIVersion("CODEX_CLI_VERSION") {
		t.Fatalf("CLI profile = %#v", profile)
	}
	if len(profile.PersistentState) != 1 || profile.PersistentState[0] != (provisioning.PersistentDirectory{
		Device: "kimi-codex-home", HostDirectory: "kimi-codex", ContainerPath: "/root/.kimi-codex",
	}) {
		t.Fatalf("persistent state = %#v", profile.PersistentState)
	}
	if profile.Instructions == nil || profile.Instructions.Path != "/root/.kimi-codex/AGENTS.md" {
		t.Fatalf("instructions = %#v", profile.Instructions)
	}
	if profile.WorkspaceSkills == nil || profile.WorkspaceSkills.WorkspaceHome != "/workspace/.kimi-codex" ||
		profile.WorkspaceSkills.HomeSkillsDir != "/root/.kimi-codex/skills" {
		t.Fatalf("workspace skills = %#v", profile.WorkspaceSkills)
	}
	if len(profile.RuntimeAssets) != 0 {
		t.Fatalf("runtime catalog must come from live discovery: %#v", profile.RuntimeAssets)
	}
}
