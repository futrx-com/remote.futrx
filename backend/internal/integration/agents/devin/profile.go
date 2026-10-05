// Package devin adapts Cognition's Devin CLI (`devin acp`) as a headless agent
// provider. The CLI speaks the Agent Client Protocol (ACP), a JSON-RPC over
// stdio protocol, so runs surface structured tool calls, usage, and sessions.
// Authentication is host-managed: an admin runs
// `devin auth login --force-manual-token-flow` once on the host and the
// credential file is synced into project containers.
package devin

import (
	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/agents/devinharness"
)

const (
	hostDevinDir               = "/root/.local/share/devin"
	hostDevinCredentials       = "/root/.local/share/devin/credentials.toml"
	containerDevinDir          = "/root/.local/share/devin"
	containerDevinCredentials  = "/root/.local/share/devin/credentials.toml"
	containerDevinInstructions = "/root/.config/devin/AGENTS.md"
	containerInstructionsHash  = "/root/.config/devin/.agents-md.sha256"
	workspaceDevinHome         = "/workspace/.devin"
)

var devinProfile = provisioning.Profile{
	ID:  string(agent.ProviderDevin),
	CLI: devinharness.NewCLISpec(),
	Credentials: provisioning.CredentialSpec{
		Name:         "devin",
		HostDir:      hostDevinDir,
		ContainerDir: containerDevinDir,
		Files: []provisioning.CredentialFile{
			// Confirmed by `devin auth status`: credentials.toml is the token
			// store at ~/.local/share/devin/credentials.toml (XDG_DATA_HOME).
			{
				HostPath:      hostDevinCredentials,
				ContainerPath: containerDevinCredentials,
				Mode:          "600",
				PushRequired:  true,
				PullRequired:  true,
			},
		},
		SeedOnLaunch: true,
	},
	PersistentState: []provisioning.PersistentDirectory{{
		Device:        "devin-home",
		HostDirectory: "devin",
		ContainerPath: containerDevinDir,
	}},
	Instructions: &provisioning.InstructionTarget{
		Path:     containerDevinInstructions,
		HashPath: containerInstructionsHash,
	},
	WorkspaceSkills: &provisioning.WorkspaceSkills{
		WorkspaceHome: workspaceDevinHome,
	},
}

// Profile returns Devin's complete provisioning policy as a defensive copy.
func Profile() provisioning.Profile {
	return devinProfile.Clone()
}
