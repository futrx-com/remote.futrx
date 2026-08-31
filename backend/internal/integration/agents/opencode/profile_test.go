package opencode

import (
	"reflect"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

func TestProfilePreservesOpenCodeProvisioningPolicy(t *testing.T) {
	want := provisioning.Profile{
		ID: "opencode",
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

	if got := Profile(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Profile() = %#v, want %#v", got, want)
	}
}

func TestProfileReturnsDefensiveCopy(t *testing.T) {
	profile := Profile()
	profile.Credentials.Files[0].HostPath = "/changed"
	profile.PersistentState[0].ContainerPath = "/changed"

	if got := Profile().Credentials.Files[0].HostPath; got != hostOpenCodeAuth() {
		t.Fatalf("Profile() retained caller mutation: %q", got)
	}
	if got := Profile().PersistentState[0].ContainerPath; got != containerOpenCodeData {
		t.Fatalf("Profile() retained persistent-state mutation: %q", got)
	}
}
