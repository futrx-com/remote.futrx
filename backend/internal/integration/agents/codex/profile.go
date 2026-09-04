package codex

import (
	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/agents/codexharness"
)

const (
	hostCodexDir  = "/root/.codex"
	hostCodexAuth = "/root/.codex/auth.json"

	containerCodexDir          = "/root/.codex"
	containerCodexAuth         = "/root/.codex/auth.json"
	containerCodexInstructions = "/root/.codex/AGENTS.md"
	containerInstructionsHash  = "/root/.claude/.agents-md.sha256"
	workspaceCodexHome         = "/workspace/.codex"
	containerCodexSkills       = "/root/.codex/skills"
)

var codexProfile = provisioning.Profile{
	ID:  string(agent.ProviderCodex),
	CLI: codexharness.NewCLISpec("Codex", "codex"),
	Credentials: provisioning.CredentialSpec{
		Name:         "codex",
		HostDir:      hostCodexDir,
		ContainerDir: containerCodexDir,
		Files: []provisioning.CredentialFile{
			{
				HostPath:      hostCodexAuth,
				ContainerPath: containerCodexAuth,
				Mode:          "600",
				PushRequired:  true,
				PullRequired:  true,
			},
		},
		SeedOnLaunch: true,
	},
	PersistentState: []provisioning.PersistentDirectory{{
		Device:        "codex-home",
		HostDirectory: "codex",
		ContainerPath: containerCodexDir,
	}},
	Instructions: &provisioning.InstructionTarget{
		Path:     containerCodexInstructions,
		HashPath: containerInstructionsHash,
	},
	WorkspaceSkills: &provisioning.WorkspaceSkills{
		WorkspaceHome: workspaceCodexHome,
		HomeSkillsDir: containerCodexSkills,
	},
}

// Profile returns Codex's complete provisioning policy.
func Profile() provisioning.Profile {
	return codexProfile.Clone()
}
