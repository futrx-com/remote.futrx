package codex

import (
	"github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/codexharness"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

const (
	hostCodexDir  = "/root/.codex"
	hostCodexAuth = "/root/.codex/auth.json"

	containerCodexDir          = "/root/.codex"
	containerCodexAuth         = "/root/.codex/auth.json"
	containerCodexInstructions = "/root/.codex/remote-system.md"
	containerInstructionsHash  = "/root/.codex/.remote-system.sha256"
	workspaceCodexHome         = "/workspace/.codex"
	containerCodexSkills       = "/root/.codex/skills"
)

var codexProfile = provisioning.Profile{
	ID:  string(harness.ProviderCodex),
	CLI: codexharness.NewCLISpec("Codex", "codex"),
	Credentials: provisioning.CredentialSpec{
		Name:         "codex",
		HostDir:      hostCodexDir,
		ContainerDir: containerCodexDir,
		Files: []provisioning.CredentialFile{
			{
				HostPath:          hostCodexAuth,
				ContainerPath:     containerCodexAuth,
				Mode:              "600",
				PushRequired:      true,
				PullRequired:      true,
				HostAuthoritative: true,
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
		Path:           containerCodexInstructions,
		HashPath:       containerInstructionsHash,
		LegacyPath:     "/root/.codex/AGENTS.md",
		LegacyHashPath: "/root/.claude/.agents-md.sha256",
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
