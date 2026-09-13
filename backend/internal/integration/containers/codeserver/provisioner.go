// Package codeserver provisions the on-demand IDE inside project containers.
package codeserver

// On-demand code-server: each project container runs its own code-server,
// socket-activated and idle-stopped (see assets/code-server-up.sh). New
// containers get it baked into the base image; EnsureCodeServer is the
// migration path for containers created before that image. Reached from the
// host edge at <slug>.code.<host> -> <slug>.lxd:8842, behind the same Google
// admin gate as the dev-URL proxy.

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	"github.com/futrx-com/remote.futrx.com/internal/shared/output"
)

//go:embed assets/code-server-up.sh
var codeServerUpScript []byte

// InstallScript returns the code-server installation program used by
// base-image builds and the on-demand migration path, with the pinned
// code-server version filled in from versions.env.
func InstallScript() []byte {
	return bytes.ReplaceAll(
		codeServerUpScript,
		[]byte("__CODE_SERVER_VERSION__"),
		[]byte(provisioning.MustCLIVersion("CODE_SERVER_VERSION")),
	)
}

// Provisioner owns installation and socket activation for the
// per-container IDE.
type Provisioner struct {
	runner         command.Runner
	publicHostname string
}

// NewProvisioner returns a code-server provisioner backed by runner.
func NewProvisioner(runner command.Runner, publicHostname ...string) *Provisioner {
	hostname := ""
	if len(publicHostname) > 0 {
		hostname = strings.TrimSuffix(strings.TrimSpace(publicHostname[0]), ".")
	}
	return &Provisioner{runner: runner, publicHostname: hostname}
}

const configurePreviewURI = `
set -euo pipefail
dropin_dir=/etc/systemd/system/code-server.service.d
dropin="$dropin_dir/10-remote-preview.conf"
install -d -m 0755 "$dropin_dir"
tmp="$(mktemp "$dropin_dir/.10-remote-preview.conf.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
printf '[Service]\nEnvironment=HOME=/root\nEnvironment=GIT_CONFIG_GLOBAL=/root/.gitconfig\nEnvironment=VSCODE_PROXY_URI=%s\nEnvironment=__VITE_ADDITIONAL_SERVER_ALLOWED_HOSTS=%s\n' \
    "$CODE_SERVER_PROXY_URI" "$VITE_ALLOWED_HOST" >"$tmp"
chmod 0644 "$tmp"
if [ -f "$dropin" ] && cmp -s "$tmp" "$dropin"; then
    exit 0
fi
mv "$tmp" "$dropin"
systemctl daemon-reload
if systemctl is-active --quiet code-server.service; then
    systemctl restart code-server.service
fi
`

// Keep browser IDE Git operations on the platform-managed credential helper.
// VS Code's GitHub extension otherwise maintains a separate OAuth session in
// container-local secret storage and asks the user to sign in again whenever
// that IDE state is recreated, even though the shared gh credential is valid.
const configureGitAuthentication = `
set -euo pipefail
settings_dir=/root/.local/share/code-server/User
settings="$settings_dir/settings.json"
install -d -m 0755 "$settings_dir"
SETTINGS_PATH="$settings" node - <<'NODE'
const fs = require("fs");
const path = process.env.SETTINGS_PATH;
let settings = {};
try {
  settings = JSON.parse(fs.readFileSync(path, "utf8"));
} catch (error) {
  if (error.code !== "ENOENT") throw error;
}
if (settings["github.gitAuthentication"] !== false) {
  settings["github.gitAuthentication"] = false;
  const temporary = path + ".tmp";
  fs.writeFileSync(temporary, JSON.stringify(settings, null, 2) + "\n", { mode: 0o600 });
  fs.renameSync(temporary, path);
}
NODE
chmod 0600 "$settings"
`

// EnsureCodeServer installs and enables the on-demand code-server stack inside
// an existing project container. Idempotent and best-effort, mirroring
// other container migration helpers. Existing units are kept, while their
// preview URL drop-in is converged on every launch. A disabled/stopped socket
// is still (re-)enabled, so a present-but-inert unit can't leave IDE routing
// silently broken.
func (p *Provisioner) Ensure(ctx context.Context, containerName, displayName, projectSlug string) error {
	// Install the units only when they're not present yet. The base image may
	// already ship them; re-running the install script is harmless but slow,
	// so skip it when the unit file exists and just (re-)enable below.
	if _, err := command.RunWithTimeout(ctx, p.runner, 10*time.Second, "exec", containerName, "--", "test", "-f", "/etc/systemd/system/code-server.socket"); err != nil {
		if out, err := command.RunWithTimeout(ctx, p.runner, 5*time.Minute, "exec", containerName, "--env", "CODE_SERVER_WS_NAME="+displayName, "--", "bash", "-c", string(InstallScript())); err != nil {
			return fmt.Errorf("install code-server: %w; output: %s", err, output.TruncateTail(out, 2000))
		}
	}

	// code-server reads this template when it builds links in the Ports tab.
	// A dedicated origin keeps root-relative Vite assets and WebSockets intact,
	// unlike the legacy /<slug>/proxy/<port>/ path.
	if p.publicHostname != "" {
		proxyURI := fmt.Sprintf("https://%s--{{port}}.dev.%s", projectSlug, p.publicHostname)
		viteAllowedHost := ".dev." + p.publicHostname
		if out, err := command.RunWithTimeout(
			ctx,
			p.runner,
			20*time.Second,
			"exec", containerName,
			"--env", "CODE_SERVER_PROXY_URI="+proxyURI,
			"--env", "VITE_ALLOWED_HOST="+viteAllowedHost,
			"--", "bash", "-c", configurePreviewURI,
		); err != nil {
			return fmt.Errorf("configure code-server preview URI: %w; output: %s", err, output.TruncateTail(out, 1000))
		}
	}

	// Converge existing containers too. The install script only runs for new
	// containers, while the OAuth-loop fix must also reach current workspaces.
	if out, err := command.RunWithTimeout(
		ctx,
		p.runner,
		10*time.Second,
		"exec", containerName,
		"--", "bash", "-c", configureGitAuthentication,
	); err != nil {
		return fmt.Errorf("configure code-server Git authentication: %w; output: %s", err, output.TruncateTail(out, 1000))
	}

	// Always enable --now: arms a freshly-installed socket, and recovers a
	// baked-but-disabled/stopped one -- the case the old file-exists check
	// reported as complete while routing was actually dead.
	if out, err := command.RunWithTimeout(ctx, p.runner, 20*time.Second, "exec", containerName, "--", "systemctl", "enable", "--now", "code-server.socket"); err != nil {
		return fmt.Errorf("enable code-server.socket: %w; output: %s", err, output.TruncateTail(out, 1000))
	}
	return nil
}
