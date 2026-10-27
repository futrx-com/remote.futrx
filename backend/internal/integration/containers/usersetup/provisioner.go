// Package usersetup restores project-owned tools inside a prepared container.
package usersetup

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	"github.com/futrx-com/remote.futrx.com/internal/shared/output"
)

const (
	setupScriptPath  = "/workspace/setup.sh"
	setupMarkerPath  = "/var/lib/remote/user-setup.sha256"
	setupRunTimeout  = 15*time.Minute + 30*time.Second
	setupOutputLimit = 64 * 1024
)

// The marker lives in the replaceable root filesystem. A failed run never
// records success, so Start retries it; replacing the container or changing
// the script also replays setup. Missing/non-executable scripts alone are no-ops.
// timeout bounds the guest process even if the host-side lxc client disconnects.
const setupCommand = `set -eu
script="$1"
marker="$2"
if [ ! -f "$script" ] || [ ! -x "$script" ]; then exit 0; fi
umask 077
mkdir -p "$(dirname "$marker")"
exec 9>"$marker.lock"
flock --exclusive 9
checksum=$(sha256sum "$script")
if [ -f "$marker" ] && [ "$(cat "$marker")" = "$checksum" ]; then exit 0; fi
bash -e "$script" 9>&-
printf '%s\n' "$checksum" > "$marker.tmp"
mv "$marker.tmp" "$marker"
`

type Provisioner struct{ runner command.Runner }

func NewProvisioner(runner command.Runner) *Provisioner { return &Provisioner{runner: runner} }

// Ensure restores setup once per script revision in each container. Explicit
// environment values also support multiline secrets, which LXD config rejects.
func (p *Provisioner) Ensure(ctx context.Context, containerName string, environment map[string]string) error {
	if !p.runner.Available() {
		return command.ErrUnavailable
	}
	args := []string{"exec", containerName, "--cwd", "/workspace", "--user", "0", "--group", "0"}
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--env", key+"="+environment[key])
	}
	args = append(args, "--", "timeout", "--kill-after=5s", "15m", "bash", "-c", setupCommand, "remote-user-setup", setupScriptPath, setupMarkerPath)
	// Setup output may contain credentials. Do not stream it into an outer log.
	ctx = command.WithCaptureLimit(command.WithOutput(ctx, io.Discard), setupOutputLimit)
	out, err := command.RunWithTimeout(ctx, p.runner, setupRunTimeout, args...)
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return fmt.Errorf("run %s: %w; %s; fix the script and Start the project to retry", setupScriptPath, err, failureOutput(out, environment))
	}
	return nil
}

func failureOutput(text string, environment map[string]string) string {
	// A capture boundary may split a secret. Avoid publishing partial values.
	if len(text) >= setupOutputLimit && len(environment) > 0 {
		return "setup output exceeded the capture limit"
	}
	var secrets []string
	for _, value := range environment {
		if value == "" {
			continue
		}
		quoted, _ := json.Marshal(value)
		secrets = append(secrets, value, string(quoted[1:len(quoted)-1]), base64.StdEncoding.EncodeToString([]byte(value)))
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		text = strings.ReplaceAll(text, secret, "[redacted]")
	}
	return strings.ToValidUTF8(output.TruncateTail(text, 4096), "")
}
