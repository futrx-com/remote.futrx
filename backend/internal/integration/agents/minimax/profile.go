package minimax

import (
	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/integration/agents/codexharness"
)

var miniMaxProfile = provisioning.Profile{
	ID: string(agent.ProviderMiniMax),
	CLI: codexharness.NewCLISpec(
		configconstants.MiniMaxCLIName,
		configconstants.MiniMaxImageLabel,
	),
	Credentials: provisioning.CredentialSpec{Name: configconstants.MiniMaxCredentialName},
	PersistentState: []provisioning.PersistentDirectory{{
		Device:        configconstants.MiniMaxPersistentDevice,
		HostDirectory: configconstants.MiniMaxHostDirectory,
		ContainerPath: configconstants.MiniMaxContainerHome,
	}},
	Instructions: &provisioning.InstructionTarget{
		Path:     configconstants.MiniMaxContainerInstructions,
		HashPath: configconstants.MiniMaxContainerInstructionsHash,
	},
	WorkspaceSkills: &provisioning.WorkspaceSkills{
		WorkspaceHome: configconstants.MiniMaxWorkspaceHome,
		HomeSkillsDir: configconstants.MiniMaxContainerSkills,
	},
}

// Profile returns MiniMax's isolated Codex runtime policy.
func Profile() provisioning.Profile {
	return miniMaxProfile.Clone()
}
