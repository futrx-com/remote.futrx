package codex

import (
	"reflect"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

func TestProfileMatchesCodexProvisioningPolicy(t *testing.T) {
	want := provisioning.Profile{
		ID: "codex",
		CLI: provisioning.CLISpec{
			Name:               "Codex",
			ImageLabel:         "codex",
			Binary:             "codex",
			VersionArgs:        []string{"--version"},
			PackageName:        "@openai/codex",
			Version:            provisioning.MustCLIVersion("CODEX_CLI_VERSION"),
			ReportVersion:      true,
			CheckVersion:       true,
			VerifyAfterInstall: true,
			InstallMode:        provisioning.InstallWithNPM,
			InstallTimeout:     5 * time.Minute,
			WaitTimeout:        2 * time.Minute,
		},
		Credentials: provisioning.CredentialSpec{
			Name:         "codex",
			HostDir:      "/root/.codex",
			ContainerDir: "/root/.codex",
			Files: []provisioning.CredentialFile{
				{
					HostPath:      "/root/.codex/auth.json",
					ContainerPath: "/root/.codex/auth.json",
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
			ContainerPath: "/root/.codex",
		}},
		Instructions: &provisioning.InstructionTarget{
			Path:     "/root/.codex/AGENTS.md",
			HashPath: "/root/.claude/.agents-md.sha256",
		},
		WorkspaceSkills: &provisioning.WorkspaceSkills{
			WorkspaceHome: "/workspace/.codex",
			HomeSkillsDir: "/root/.codex/skills",
		},
	}

	if got := Profile(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Profile() = %#v, want %#v", got, want)
	}
}
