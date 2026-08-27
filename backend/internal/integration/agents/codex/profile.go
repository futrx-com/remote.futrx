package codex

import (
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
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
	ID: string(agent.ProviderCodex),
	CLI: provisioning.CLISpec{
		Name:               "Codex",
		ImageLabel:         "codex",
		Binary:             "codex",
		VersionArgs:        []string{"--version"},
		PackageName:        "@openai/codex",
		Version:            provisioning.MustCLIVersion("CODEX_CLI_VERSION"),
		CheckVersion:       true,
		VerifyAfterInstall: true,
		ReportVersion:      true,
		InstallMode:        provisioning.InstallWithNPM,
		InstallTimeout:     5 * time.Minute,
		WaitTimeout:        2 * time.Minute,
	},
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
