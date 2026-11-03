package browser

// Browser-script provisioning: pushes the generic Playwright wrapper at
// /workspace/scripts/browser.mjs and seeds an empty config file at
// /workspace/.agents/browser-auth.json. Both are workspace-resident so
// they survive container deletes; the host re-pushes the script whenever
// the embedded template changes (sha256 marker, same pattern as AGENTS.md).
//
// The script reads .agents/browser-auth.json to know which cookie to
// attach per host; the agent edits that file when adding a new site, and
// the user pastes the cookie value into the Secrets UI under the named
// env var. See agent/provisioning/assets/AGENTS.md for the user-facing recipe.

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

//go:embed assets/browser.mjs
var browserScriptTemplate []byte

const (
	containerBrowserScript     = "/workspace/scripts/browser.mjs"
	containerBrowserAuthConfig = "/workspace/.agents/browser-auth.json"
	containerBrowserScriptHash = "/workspace/.agents/.browser-mjs.sha256"
)

// EnsureBrowserScript pushes the generic Playwright wrapper into the
// workspace and seeds an empty .agents/browser-auth.json if missing.
// Idempotent: the script is only re-pushed when its embedded content
// changes (sha256 marker stored alongside the config).
func (a *Adapter) EnsureScript(ctx context.Context, containerName string) error {
	if !a.runner.Available() {
		return command.ErrUnavailable
	}

	// Always ensure the directories + config file exist (cheap, idempotent).
	// We chmod 755 on dirs so the unprivileged container-root user can
	// traverse them; the host bind-mount preserves the uid 1000000 owner.
	_, err := command.RunWithTimeout(ctx, a.runner, 30*time.Second, "exec", containerName, "--", "sh", "-c", `set -eu
mkdir -p /workspace/scripts /workspace/.agents /workspace/.browser
chmod 755 /workspace/scripts /workspace/.agents /workspace/.browser
if [ ! -f /workspace/.agents/browser-auth.json ]; then
  printf '{}\n' > /workspace/.agents/browser-auth.json
  chmod 644 /workspace/.agents/browser-auth.json
fi`)
	if err != nil {
		return fmt.Errorf("seed browser dirs: %w", err)
	}

	return a.publisher.Push(ctx, containerName, []byte(newBrowserAssetRenderer().render(string(browserScriptTemplate))), containerBrowserScriptHash, "755", containerBrowserScript)
}
