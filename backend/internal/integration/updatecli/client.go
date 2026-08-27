// Package updatecli shells out to git and the repo's installer scripts on
// behalf of the self-update flow. Deliberately policy-free: the selfupdate
// service decides what runs and when; this package only knows how.
package updatecli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const lsRemoteTimeout = 30 * time.Second

type Client struct{}

func New() Client { return Client{} }

// ListRemoteTags returns the tag names on the checkout's origin remote,
// reusing whatever credentials are embedded in its git config. Annotated
// tags appear once (the "^{}" peel lines are folded into their tag).
func (Client) ListRemoteTags(ctx context.Context, installDir string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, lsRemoteTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", installDir, "ls-remote", "--tags", "origin").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("git ls-remote: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("git ls-remote: %w", err)
	}
	seen := map[string]bool{}
	var tags []string
	for _, line := range strings.Split(string(out), "\n") {
		_, ref, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(ref), "refs/tags/")
		name = strings.TrimSuffix(name, "^{}")
		if name == "" || strings.HasPrefix(name, "refs/") || seen[name] {
			continue
		}
		seen[name] = true
		tags = append(tags, name)
	}
	return tags, nil
}

// StartUpdater launches the selected release script in its own session so
// that the systemd restart cannot kill it (the unit runs with
// KillMode=process). Output streams to logPath; when the run finishes, its
// exit code is written to donePath — that file, not the process, is the
// durable record of the outcome, because the backend that spawned the run is
// usually replaced before the run ends.
func (Client) StartUpdater(installDir, tag, kind, logPath, donePath string) (int, error) {
	if kind != "application" && kind != "infrastructure" {
		return 0, fmt.Errorf("unknown update kind: %s", kind)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	defer logFile.Close()

	// Values reach the script as positional parameters, never by string
	// interpolation.
	const script = `case "$4" in
application) FUTRX_INSTALL_DIR="$1" bash "$1/infra/deploy-app.sh" "--ref=$2" ;;
infrastructure) FUTRX_INSTALL_DIR="$1" bash "$1/infra/update.sh" "--ref=$2" ;;
*) echo "unknown update kind: $4" >&2; exit 2 ;;
esac
status=$?
printf '{"exitCode":%d,"finishedAt":%d}\n' "$status" "$(date +%s)" > "$3.tmp" && mv "$3.tmp" "$3"
exit "$status"`
	cmd := exec.Command("bash", "-c", script, "self-update", installDir, tag, donePath, kind)
	cmd.Dir = installDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	// Reap the child if this process outlives the run; after a restart the
	// orphan is init's problem and donePath carries the outcome.
	go func() { _ = cmd.Wait() }()
	return cmd.Process.Pid, nil
}

// ProcessAlive reports whether pid is still running.
func (Client) ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
