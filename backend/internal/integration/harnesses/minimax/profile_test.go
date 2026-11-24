package minimax

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

func TestProfileUsesIsolatedCodexHome(t *testing.T) {
	profile := Profile()
	if profile.ID != "minimax" || profile.CLI.Binary != "codex" ||
		profile.CLI.PackageName != "@openai/codex" ||
		profile.CLI.Version != provisioning.MustCLIVersion("CODEX_CLI_VERSION") {
		t.Fatalf("CLI profile = %#v", profile)
	}
	if len(profile.PersistentState) != 1 || profile.PersistentState[0] != (provisioning.PersistentDirectory{
		Device: "minimax-home", HostDirectory: "minimax", ContainerPath: "/root/.minimax",
	}) {
		t.Fatalf("persistent state = %#v", profile.PersistentState)
	}
	if profile.LegacyInstructions == nil || profile.LegacyInstructions.Path != "/root/.minimax/AGENTS.md" {
		t.Fatalf("instructions = %#v", profile.LegacyInstructions)
	}
	if profile.WorkspaceSkills == nil || profile.WorkspaceSkills.WorkspaceHome != "/workspace/.minimax" ||
		profile.WorkspaceSkills.HomeSkillsDir != "/root/.minimax/skills" {
		t.Fatalf("workspace skills = %#v", profile.WorkspaceSkills)
	}
	if len(profile.RuntimeAssets) != 0 {
		t.Fatalf("runtime catalog must come from live discovery: %#v", profile.RuntimeAssets)
	}
}
