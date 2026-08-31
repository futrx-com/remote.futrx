package opencode

import (
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

const (
	containerOpenCodeData  = "/root/.local/share/opencode"
	containerOpenCodeAuth  = containerOpenCodeData + "/auth.json"
	missingAuthJSONFormat  = "opencode not authenticated — run `opencode auth login` on the host or in container %s"
	opencodeDataDeviceName = "opencode-data"
)

var opencodeProfile = provisioning.Profile{
	ID: string(agent.ProviderOpenCode),
	CLI: provisioning.CLISpec{
		Name:               "OpenCode",
		ImageLabel:         "opencode",
		Binary:             "opencode",
		VersionArgs:        []string{"--version"},
		PackageName:        "opencode-ai",
		Version:            provisioning.MustCLIVersion("OPENCODE_VERSION"),
		ReportVersion:      true,
		CheckVersion:       true,
		VerifyAfterInstall: true,
		InstallMode:        provisioning.InstallWithNPM,
		InstallTimeout:     5 * time.Minute,
		WaitTimeout:        2 * time.Minute,
	},
	Credentials: provisioning.CredentialSpec{
		Name:         "opencode",
		HostDir:      hostOpenCodeData(),
		ContainerDir: containerOpenCodeData,
		Files: []provisioning.CredentialFile{
			{
				HostPath:      hostOpenCodeAuth(),
				ContainerPath: containerOpenCodeAuth,
				Mode:          "600",
				PushRequired:  true,
				PullRequired:  true,
			},
		},
		SeedOnLaunch: true,
	},
	PersistentState: []provisioning.PersistentDirectory{{
		Device:        opencodeDataDeviceName,
		HostDirectory: "opencode",
		ContainerPath: containerOpenCodeData,
	}},
}

// Profile returns OpenCode's provisioning policy. The returned value is a
// defensive copy so application wiring can compose profiles without mutating
// the provider's definition.
func Profile() provisioning.Profile {
	return opencodeProfile.Clone()
}
