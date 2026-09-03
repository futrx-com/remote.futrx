package kimi

import (
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

const (
	hostKimiCredentialsDir   = "/root/.kimi-code/credentials"
	containerCredentialsDir  = containerKimiHome + "/credentials"
	missingCredentialsFormat = "kimi not authenticated — run `kimi login` on the host or in container %s"
)

var kimiProfile = provisioning.Profile{
	ID: string(agent.ProviderKimi),
	CLI: provisioning.CLISpec{
		Name:               "kimi",
		ImageLabel:         "kimi-code",
		Binary:             "kimi",
		VersionArgs:        []string{"--version"},
		PackageName:        "@moonshot-ai/kimi-code",
		Version:            provisioning.MustCLIVersion("KIMI_CODE_VERSION"),
		ReportVersion:      true,
		CheckVersion:       true,
		VerifyAfterInstall: true,
		InstallMode:        provisioning.InstallWithImageRepair,
		InstallTimeout:     8 * time.Minute,
		WaitTimeout:        5 * time.Minute,
	},
	Credentials: provisioning.CredentialSpec{
		Name: "kimi",
		Directory: &provisioning.CredentialDirectory{
			HostPath:                 hostKimiCredentialsDir,
			ContainerPath:            containerCredentialsDir,
			ContainerDirs:            []string{containerKimiHome, containerCredentialsDir},
			AllowContainerOnly:       true,
			MissingErrorFormat:       missingCredentialsFormat,
			SyncOnlyWhenHostHasFiles: true,
			SyncUnavailableIsNoop:    true,
		},
		SeedOnLaunch: false,
	},
	PersistentState: []provisioning.PersistentDirectory{{
		Device:        "kimi-home",
		HostDirectory: "kimi",
		ContainerPath: containerKimiHome,
	}},
}

// Profile returns Kimi's provisioning policy. The returned value is a
// defensive copy so application wiring can compose profiles without mutating
// the provider's definition.
func Profile() provisioning.Profile {
	return kimiProfile.Clone()
}
