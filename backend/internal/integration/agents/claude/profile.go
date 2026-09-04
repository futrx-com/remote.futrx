package claude

import (
	_ "embed"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

const (
	browserMCPConfigPath = "/workspace/.browser-gui/mcp-claude.json"
	browserMCPConfigHash = "/workspace/.browser-gui/.mcp-claude.sha256"
	browserGUIDir        = "/workspace/.browser-gui"
)

//go:embed assets/mcp.json
var browserMCPConfig []byte

// Profile defines Claude-owned CLI and project-container provisioning policy.
// Environment integrations apply it without knowing Claude's package,
// credentials, or filesystem layout.
func Profile() provisioning.Profile {
	return provisioning.Profile{
		ID: string(agent.ProviderClaude),
		CLI: provisioning.CLISpec{
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
		},
		Credentials: provisioning.CredentialSpec{
			Name:          "claude",
			HostDir:       "/root/.claude",
			ContainerDir:  "/root/.claude",
			LegacyDevices: []string{"claude-auth"},
			Files: []provisioning.CredentialFile{
				{
					HostPath:      "/root/.claude.json",
					ContainerPath: "/root/.claude.json",
					Mode:          "600",
					PushRequired:  true,
					PullRequired:  true,
				},
				{
					HostPath:      "/root/.claude/.credentials.json",
					ContainerPath: "/root/.claude/.credentials.json",
					Mode:          "600",
					PullRequired:  true,
				},
			},
			SeedOnLaunch: true,
		},
		PersistentState: []provisioning.PersistentDirectory{{
			Device:        "claude-home",
			HostDirectory: "claude",
			ContainerPath: "/root/.claude",
		}},
		Instructions: &provisioning.InstructionTarget{
			Path:     "/root/.claude/CLAUDE.md",
			HashPath: "/root/.claude/.agents-md.sha256",
		},
		WorkspaceSkills: &provisioning.WorkspaceSkills{
			WorkspaceHome: "/workspace/.claude",
		},
		BrowserMCPTemplates: []provisioning.TemplateFile{
			{
				Content:       append([]byte(nil), browserMCPConfig...),
				Path:          browserMCPConfigPath,
				HashPath:      browserMCPConfigHash,
				Mode:          "644",
				Directory:     browserGUIDir,
				DirectoryMode: "755",
			},
		},
	}
}
