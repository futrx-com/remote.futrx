// Package templates translates project-template provisioning into container
// runtime commands.
package templates

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

const (
	queryTimeout = 15 * time.Second
	pushTimeout  = 60 * time.Second
)

// Adapter is the command adapter for the project-template workflow.
type Adapter struct {
	runner command.Runner
}

// NewAdapter returns a template runtime backed by runner.
func NewAdapter(runner command.Runner) *Adapter {
	return &Adapter{runner: runner}
}

func (a *Adapter) Available() bool {
	return a.runner.Available()
}

// FileExists reports whether path exists inside the container.
//
// Two failures mean "absent" rather than "broken": a non-zero exit with no
// output is `test -e` answering no, and a container that does not exist
// obviously holds no marker — the project status endpoint polls this for
// containers that were never created, and neither case deserves an error.
func (a *Adapter) FileExists(ctx context.Context, containerName, path string) (bool, error) {
	out, err := command.RunWithTimeout(
		ctx, a.runner, queryTimeout,
		"exec", containerName, "--", "test", "-e", path,
	)
	if err == nil {
		return true, nil
	}
	if strings.TrimSpace(out) == "" || missingInstance(out) {
		return false, nil
	}
	return false, fmt.Errorf("probe %s in %s: %w; output: %s", path, containerName, err, out)
}

// missingInstance recognizes LXD's "Error: Instance not found".
func missingInstance(output string) bool {
	return strings.Contains(strings.ToLower(output), "instance not found")
}

// EnsureDirectory creates path (and parents) inside the container.
func (a *Adapter) EnsureDirectory(ctx context.Context, containerName, path string) error {
	if out, err := command.RunWithTimeout(
		ctx, a.runner, queryTimeout,
		"exec", containerName, "--", "install", "-d", "-m", "755", path,
	); err != nil {
		return fmt.Errorf("create %s in %s: %w; output: %s", path, containerName, err, out)
	}
	return nil
}

// PushFile writes content to an absolute container path.
func (a *Adapter) PushFile(
	ctx context.Context,
	containerName string,
	content []byte,
	target, fileMode string,
) error {
	tmp, err := os.CreateTemp("", "futrx-template-seed-*")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write seed: %w", err)
	}
	tmp.Close()

	if out, err := command.RunWithTimeout(
		ctx, a.runner, pushTimeout,
		"file", "push", "--mode="+fileMode, tmp.Name(), containerName+target,
	); err != nil {
		return fmt.Errorf("push %s to %s: %w; output: %s", target, containerName, err, out)
	}
	return nil
}

// RunScript executes a bash program inside the container. The caller owns the
// deadline: a stack install runs for minutes.
func (a *Adapter) RunScript(ctx context.Context, containerName, script string) (string, error) {
	return a.runner.Run(ctx, "exec", containerName, "--", "bash", "-c", script)
}

// ImageExists reports whether an image alias is published on this host.
func (a *Adapter) ImageExists(ctx context.Context, alias string) (bool, error) {
	out, err := command.RunWithTimeout(
		ctx, a.runner, queryTimeout,
		"image", "list", "--format", "csv", alias,
	)
	if err != nil {
		return false, fmt.Errorf("list image %s: %w; output: %s", alias, err, out)
	}
	// `lxc image list <alias>` filters by fingerprint OR alias and prints one
	// CSV row per match. An alias that resolves nothing prints nothing.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), alias+",") {
			return true, nil
		}
	}
	return false, nil
}
