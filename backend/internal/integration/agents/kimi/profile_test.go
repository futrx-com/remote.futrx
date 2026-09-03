package kimi

import (
	"reflect"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

func TestProfilePreservesKimiProvisioningPolicy(t *testing.T) {
	want := provisioning.Profile{
		ID: "kimi",
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
				HostPath:                 "/root/.kimi-code/credentials",
				ContainerPath:            "/root/.kimi-code/credentials",
				ContainerDirs:            []string{"/root/.kimi-code", "/root/.kimi-code/credentials"},
				AllowContainerOnly:       true,
				MissingErrorFormat:       "kimi not authenticated — run `kimi login` on the host or in container %s",
				SyncOnlyWhenHostHasFiles: true,
				SyncUnavailableIsNoop:    true,
			},
			SeedOnLaunch: false,
		},
		PersistentState: []provisioning.PersistentDirectory{{
			Device:        "kimi-home",
			HostDirectory: "kimi",
			ContainerPath: "/root/.kimi-code",
		}},
	}

	if got := Profile(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Profile() = %#v, want %#v", got, want)
	}
}

func TestProfileReturnsDefensiveCopy(t *testing.T) {
	profile := Profile()
	profile.Credentials.Directory.ContainerDirs[0] = "/changed"
	profile.PersistentState[0].ContainerPath = "/changed"

	if got := Profile().Credentials.Directory.ContainerDirs[0]; got != containerKimiHome {
		t.Fatalf("Profile() retained caller mutation: %q", got)
	}
	if got := Profile().PersistentState[0].ContainerPath; got != containerKimiHome {
		t.Fatalf("Profile() retained persistent-state mutation: %q", got)
	}
}
