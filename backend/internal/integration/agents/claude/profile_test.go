package claude

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

func TestProfilePreservesClaudeProvisioningPolicy(t *testing.T) {
	profile := Profile()

	if profile.ID != "claude" {
		t.Fatalf("profile ID = %q", profile.ID)
	}
	wantCLI := provisioning.CLISpec{
		Name:               "Claude Code",
		ImageLabel:         "claude-code",
		Binary:             "claude",
		VersionArgs:        []string{"--version"},
		PackageName:        "@anthropic-ai/claude-code",
		Version:            provisioning.MustCLIVersion("CLAUDE_CODE_VERSION"),
		ReportVersion:      true,
		CheckVersion:       true,
		VerifyAfterInstall: true,
		InstallMode:        provisioning.InstallWithNPM,
		InstallTimeout:     5 * time.Minute,
		WaitTimeout:        2 * time.Minute,
	}
	if !reflect.DeepEqual(profile.CLI, wantCLI) {
		t.Fatalf("CLI profile = %#v, want %#v", profile.CLI, wantCLI)
	}

	credentials := profile.Credentials
	if credentials.Name != "claude" || credentials.HostDir != "/root/.claude" || credentials.ContainerDir != "/root/.claude" {
		t.Fatalf("credential roots = %#v", credentials)
	}
	if !credentials.SeedOnLaunch {
		t.Fatal("Claude credentials must be seeded on launch")
	}
	if len(credentials.LegacyDevices) != 1 || credentials.LegacyDevices[0] != "claude-auth" {
		t.Fatalf("legacy devices = %#v", credentials.LegacyDevices)
	}
	if len(credentials.Files) != 2 {
		t.Fatalf("credential files = %#v", credentials.Files)
	}
	if len(profile.PersistentState) != 1 || profile.PersistentState[0] != (provisioning.PersistentDirectory{
		Device: "claude-home", HostDirectory: "claude", ContainerPath: "/root/.claude",
	}) {
		t.Fatalf("persistent state = %#v", profile.PersistentState)
	}
	if got := credentials.Files[0]; got.HostPath != "/root/.claude.json" || got.ContainerPath != "/root/.claude.json" || got.Mode != "600" || !got.PushRequired || !got.PullRequired {
		t.Fatalf("primary credential = %#v", got)
	}
	if got := credentials.Files[1]; got.HostPath != "/root/.claude/.credentials.json" || got.ContainerPath != "/root/.claude/.credentials.json" || got.Mode != "600" || got.PushRequired || !got.PullRequired {
		t.Fatalf("refresh credential = %#v", got)
	}

	if profile.Instructions == nil || profile.Instructions.Path != "/root/.claude/CLAUDE.md" || profile.Instructions.HashPath != "/root/.claude/.agents-md.sha256" {
		t.Fatalf("instruction target = %#v", profile.Instructions)
	}
	if profile.WorkspaceSkills == nil || profile.WorkspaceSkills.WorkspaceHome != "/workspace/.claude" || profile.WorkspaceSkills.HomeSkillsDir != "" {
		t.Fatalf("workspace skills = %#v", profile.WorkspaceSkills)
	}
	if len(profile.BrowserMCPTemplates) != 1 {
		t.Fatalf("browser MCP templates = %#v", profile.BrowserMCPTemplates)
	}
	template := profile.BrowserMCPTemplates[0]
	if template.Path != browserMCPConfigPath || template.HashPath != browserMCPConfigHash || template.Mode != "644" || template.Directory != browserGUIDir || template.DirectoryMode != "755" {
		t.Fatalf("browser MCP template = %#v", template)
	}
	if !strings.Contains(string(template.Content), `"@playwright/mcp"`) {
		t.Fatalf("browser MCP content = %q", template.Content)
	}
}
