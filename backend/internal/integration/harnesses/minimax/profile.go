package minimax

import (
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/codexharness"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

var miniMaxProfile = provisioning.Profile{
	ID: string(harness.ProviderMiniMax),
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
	LegacyInstructions: &provisioning.InstructionTarget{
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
