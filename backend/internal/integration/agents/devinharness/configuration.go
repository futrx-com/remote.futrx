// Package devinharness owns the Devin CLI installation policy and, in later
// phases, the ACP protocol adapter used by the devin provider package. For now
// only the CLI spec is exposed so the host installer can converge the binary.
package devinharness

import (
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

// manifestBaseURL is the versioned Devin CLI manifest endpoint. The install
// script fetches it to resolve per-platform archive URLs and SHA-256 checksums.
const manifestBaseURL = "https://static.devin.ai/cli"

// NewCLISpec returns the Devin CLI installation policy. Devin ships a standalone
// binary distributed through a versioned manifest with per-platform SHA-256
// checksums, so the install script fetches the manifest at install time rather
// than relying on hardcoded checksum pins.
func NewCLISpec() provisioning.CLISpec {
	version := provisioning.MustCLIVersion("DEVIN_CLI_VERSION")
	return provisioning.CLISpec{
		Name:               "devin",
		ImageLabel:         "devin",
		Binary:             "devin",
		VersionArgs:        []string{"--version"},
		Version:            version,
		ReportVersion:      true,
		CheckVersion:       true,
		VerifyAfterInstall: true,
		InstallMode:        provisioning.InstallWithScript,
		InstallScript:      installScript(version),
		InstallTimeout:     8 * time.Minute,
		WaitTimeout:        5 * time.Minute,
	}
}

// installScript downloads the pinned Devin CLI release for the target
// execution environment's architecture, verifies its manifest-published SHA-256
// checksum, and installs the binary to the path supplied by the execution
// environment. Container builds retain the /usr/local/bin/devin default. It
// never consults Devin's moving latest manifest.
func installScript(version string) string {
	return fmt.Sprintf(`set -euo pipefail
install_path="${FUTRX_HOST_CLI_INSTALL_PATH:-/usr/local/bin/devin}"
if [ -x "$install_path" ] && [ "$("$install_path" --version 2>/dev/null)" = %[1]q ]; then
    exit 0
fi
case "$(uname -m)" in
    x86_64|amd64) target="x86_64-unknown-linux" ;;
    aarch64|arm64) target="aarch64-unknown-linux" ;;
    *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac
manifest=$(curl -fsSL "%[2]s/%[1]s/manifest.json")
url=$(echo "$manifest" | grep -o "\"$target\"[^}]*}" | grep -o '"url":"[^"]*"' | sed 's/.*"\([^"]*\)"$/\1/')
sha256=$(echo "$manifest" | grep -o "\"$target\"[^}]*}" | grep -o '"sha256":"[^"]*"' | sed 's/.*"\([^"]*\)"$/\1/')
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$url" -o "$tmp/devin.tar.gz"
echo "${sha256}  $tmp/devin.tar.gz" | sha256sum -c - >/dev/null
tar -xzf "$tmp/devin.tar.gz" -C "$tmp"
install -d -m 0755 "$(dirname "$install_path")"
install -m 0755 "$tmp/bin/devin" "$install_path"
"$install_path" --version`, version, manifestBaseURL)
}
