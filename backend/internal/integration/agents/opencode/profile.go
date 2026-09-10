package opencode

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

const (
	containerOpenCodeData  = "/root/.local/share/opencode"
	containerOpenCodeAuth  = containerOpenCodeData + "/auth.json"
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

// hostOpenCodeData resolves the OpenCode data directory (XDG_DATA_HOME aware).
func hostOpenCodeData() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "opencode")
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "share", "opencode")
	}
	return "/root/.local/share/opencode"
}

func hostOpenCodeAuth() string {
	return filepath.Join(hostOpenCodeData(), "auth.json")
}

func opencodeEnv(base []string) []string {
	// OpenCode resolves its data directory from XDG_DATA_HOME; forward it so
	// the integration writes and reads consistently.
	for _, env := range base {
		if strings.HasPrefix(env, "XDG_DATA_HOME=") {
			return base
		}
	}
	return append(base, "XDG_DATA_HOME="+filepath.Dir(hostOpenCodeData()))
}
