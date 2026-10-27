package kimicodex

import (
	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/integration/agents/codexharness"
)

var kimiCodexProfile = provisioning.Profile{
	ID: string(agent.ProviderKimiCodex),
	CLI: codexharness.NewCLISpec(
		configconstants.KimiCodexCLIName,
		configconstants.KimiCodexImageLabel,
	),
	Credentials: provisioning.CredentialSpec{Name: configconstants.KimiCodexCredentialName},
	PersistentState: []provisioning.PersistentDirectory{{
		Device:        configconstants.KimiCodexPersistentDevice,
		HostDirectory: configconstants.KimiCodexHostDirectory,
		ContainerPath: configconstants.KimiCodexContainerHome,
	}},
	Instructions: &provisioning.InstructionTarget{
		Path:     configconstants.KimiCodexContainerInstructions,
		HashPath: configconstants.KimiCodexContainerInstructionsHash,
	},
	WorkspaceSkills: &provisioning.WorkspaceSkills{
		WorkspaceHome: configconstants.KimiCodexWorkspaceHome,
		HomeSkillsDir: configconstants.KimiCodexContainerSkills,
	},
}

// Profile returns Kimi's isolated Codex runtime policy.
func Profile() provisioning.Profile {
	return kimiCodexProfile.Clone()
}
