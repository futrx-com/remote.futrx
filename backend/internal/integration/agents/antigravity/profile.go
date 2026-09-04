// Package antigravity adapts Google's Antigravity CLI (`agy`) as a headless
// agent provider. The CLI has a print mode but no structured event stream, so
// runs surface as plain streaming text; conversation ids are recovered from
// the CLI's on-disk brain directory because print mode never emits them
// (github.com/google-antigravity/antigravity-cli issue #7).
package antigravity

import (
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

// containerAgentHome is HOME inside a project container: agy keeps its state
// (auth fallback tokens, conversation brain) under $HOME/.gemini.
const containerAgentHome = "/root"

// stateDirUnderHome is where agy stores conversations and headless auth state.
const stateDirUnderHome = ".gemini/antigravity-cli"

// releaseBaseURL contains version-addressed Antigravity CLI assets.
const releaseBaseURL = "https://github.com/google-antigravity/antigravity-cli/releases/download"

var antigravityProfile = provisioning.Profile{
	ID: string(agent.ProviderAntigravity),
	CLI: provisioning.CLISpec{
		Name:               "antigravity",
		ImageLabel:         "antigravity",
		Binary:             "agy",
		VersionArgs:        []string{"--version"},
		Version:            provisioning.MustCLIVersion("ANTIGRAVITY_CLI_VERSION"),
		ReportVersion:      true,
		CheckVersion:       true,
		VerifyAfterInstall: true,
		InstallMode:        provisioning.InstallWithScript,
		InstallScript: installScript(
			provisioning.MustCLIVersion("ANTIGRAVITY_CLI_VERSION"),
			provisioning.MustPin("ANTIGRAVITY_LINUX_X64_SHA512"),
			provisioning.MustPin("ANTIGRAVITY_LINUX_ARM64_SHA512"),
		),
		InstallTimeout: 8 * time.Minute,
		WaitTimeout:    5 * time.Minute,
	},
	// No credential sync: agy stores auth in the OS keyring on desktops and in
	// per-home fallback files on headless systems, with no stable documented
	// token subpath to move between host and container. Sign-in is per
	// workspace: run `agy` once in the chat terminal and complete the URL +
	// code flow. Runs without credentials fail with agy's own sign-in message.
	Credentials: provisioning.CredentialSpec{Name: "antigravity"},
	PersistentState: []provisioning.PersistentDirectory{{
		Device:        "antigravity-home",
		HostDirectory: "antigravity",
		ContainerPath: "/root/" + stateDirUnderHome,
	}},
}

// Profile returns Antigravity's provisioning policy as a defensive copy.
func Profile() provisioning.Profile {
	return antigravityProfile.Clone()
}

// installScript downloads the pinned agy release for the target execution
// environment's architecture, verifies its repository-pinned checksum, and installs
// the path supplied by the execution environment. Container builds retain the
// /usr/local/bin/agy default. It never consults Antigravity's moving latest
// manifest.
func installScript(version, linuxX64SHA512, linuxARM64SHA512 string) string {
	return fmt.Sprintf(`set -euo pipefail
install_path="${FUTRX_HOST_CLI_INSTALL_PATH:-/usr/local/bin/agy}"
if [ -x "$install_path" ] && [ "$("$install_path" --version 2>/dev/null)" = %[1]q ]; then
    exit 0
fi
case "$(uname -m)" in
    x86_64|amd64)
        asset="agy_cli_linux_x64.tar.gz"
        sha512=%[3]q
        ;;
    aarch64|arm64)
        asset="agy_cli_linux_arm64.tar.gz"
        sha512=%[4]q
        ;;
    *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac
url="%[2]s/%[1]s/${asset}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$url" -o "$tmp/agy.tar.gz"
echo "${sha512}  $tmp/agy.tar.gz" | sha512sum -c - >/dev/null
tar -xzf "$tmp/agy.tar.gz" -C "$tmp" antigravity
install -d -m 0755 "$(dirname "$install_path")"
install -m 0755 "$tmp/antigravity" "$install_path"
"$install_path" --version`, version, releaseBaseURL, linuxX64SHA512, linuxARM64SHA512)
}
