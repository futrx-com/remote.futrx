package devin

import (
	"reflect"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/agents/devinharness"
)

func TestProfilePreservesDevinProvisioningPolicy(t *testing.T) {
	profile := Profile()

	if profile.ID != "devin" {
		t.Fatalf("profile ID = %q", profile.ID)
	}

	wantCLI := devinharness.NewCLISpec()
	if !reflect.DeepEqual(profile.CLI, wantCLI) {
		t.Fatalf("CLI profile = %#v, want %#v", profile.CLI, wantCLI)
	}
	if profile.CLI.Version != provisioning.MustCLIVersion("DEVIN_CLI_VERSION") {
		t.Fatalf("CLI version = %q", profile.CLI.Version)
	}
	if profile.CLI.InstallMode != provisioning.InstallWithScript {
		t.Fatalf("install mode = %q", profile.CLI.InstallMode)
	}
	if profile.CLI.InstallTimeout != 8*time.Minute || profile.CLI.WaitTimeout != 5*time.Minute {
		t.Fatalf("timeouts = (%s, %s)", profile.CLI.InstallTimeout, profile.CLI.WaitTimeout)
	}

	credentials := profile.Credentials
	if credentials.Name != "devin" ||
		credentials.HostDir != "/root/.local/share/devin" ||
		credentials.ContainerDir != "/root/.local/share/devin" {
		t.Fatalf("credential roots = %#v", credentials)
	}
	if !credentials.SeedOnLaunch {
		t.Fatal("Devin credentials must be seeded on launch")
	}
	if len(credentials.Files) != 1 {
		t.Fatalf("credential files = %#v", credentials.Files)
	}
	cred := credentials.Files[0]
	if cred.HostPath != "/root/.local/share/devin/credentials.toml" ||
		cred.ContainerPath != "/root/.local/share/devin/credentials.toml" ||
		cred.Mode != "600" || !cred.PushRequired || !cred.PullRequired {
		t.Fatalf("credential file = %#v", cred)
	}

	if len(profile.PersistentState) != 1 || profile.PersistentState[0] != (provisioning.PersistentDirectory{
		Device: "devin-home", HostDirectory: "devin", ContainerPath: "/root/.local/share/devin",
	}) {
		t.Fatalf("persistent state = %#v", profile.PersistentState)
	}

	if profile.Instructions == nil ||
		profile.Instructions.Path != "/root/.config/devin/AGENTS.md" ||
		profile.Instructions.HashPath != "/root/.config/devin/.agents-md.sha256" {
		t.Fatalf("instruction target = %#v", profile.Instructions)
	}

	if profile.WorkspaceSkills == nil || profile.WorkspaceSkills.WorkspaceHome != "/workspace/.devin" {
		t.Fatalf("workspace skills = %#v", profile.WorkspaceSkills)
	}
}

func TestProfileReturnsDefensiveCopy(t *testing.T) {
	profile := Profile()
	profile.Credentials.Files[0].HostPath = "/changed"
	profile.PersistentState[0].ContainerPath = "/changed"

	if got := Profile().Credentials.Files[0].HostPath; got != hostDevinCredentials {
		t.Fatalf("Profile() retained credential mutation: %q", got)
	}
	if got := Profile().PersistentState[0].ContainerPath; got != containerDevinDir {
		t.Fatalf("Profile() retained persistent-state mutation: %q", got)
	}
}
