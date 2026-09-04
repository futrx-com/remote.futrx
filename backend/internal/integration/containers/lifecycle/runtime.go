package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

const (
	// TODO: We will need to investigate how to hook into a callback instead of increasing the timeout
	launchTimeout  = 10 * time.Minute
	migrateTimeout = 5 * time.Minute
	startTimeout   = 30 * time.Second
	stopTimeout    = 30 * time.Second
	restartTimeout = 60 * time.Second
	deleteTimeout  = 5 * time.Minute
	queryTimeout   = 10 * time.Second
)

// Client translates lifecycle operations into LXD CLI calls.
type Client struct {
	runner command.Runner
}

type deleteAttempt struct {
	output   string
	err      error
	timedOut bool
}

func NewClient(runner command.Runner) *Client {
	return &Client{runner: runner}
}

func (c *Client) Available() bool {
	return c.runner.Available()
}

// Init creates a stopped container. Project storage is attached before Start,
// so a container can never become RUNNING without its durable mounts.
func (c *Client) Init(ctx context.Context, image, containerName string) error {
	if out, err := command.RunWithTimeout(ctx, c.runner, launchTimeout, "init", image, containerName); err != nil {
		return fmt.Errorf("lxc init: %w; output: %s", err, out)
	}
	return nil
}

func (c *Client) Disk(ctx context.Context, container, deviceName string) (string, string, bool, error) {
	out, err := command.RunWithTimeout(ctx, c.runner, queryTimeout, "query", "/1.0/instances/"+container)
	if err != nil {
		return "", "", false, fmt.Errorf("show container devices: %w; output: %s", err, out)
	}
	var config struct {
		Devices map[string]map[string]string `json:"devices"`
	}
	if err := json.Unmarshal([]byte(out), &config); err != nil {
		return "", "", false, fmt.Errorf("decode container devices: %w", err)
	}
	device, ok := config.Devices[deviceName]
	if !ok {
		return "", "", false, nil
	}
	return device["source"], device["path"], true, nil
}

func (c *Client) AttachDisk(ctx context.Context, container, deviceName, hostSrc, containerPath string, readonly bool) error {
	args := []string{
		"config", "device", "add", container, deviceName, "disk",
		"source=" + hostSrc,
		"path=" + containerPath,
	}
	if readonly {
		args = append(args, "readonly=true")
	}
	if out, err := command.RunWithTimeout(ctx, c.runner, queryTimeout, args...); err != nil {
		return fmt.Errorf("lxc config device add %s: %w; output: %s", deviceName, err, out)
	}
	return nil
}

func (c *Client) RemoveDevice(ctx context.Context, container, deviceName string) error {
	if out, err := command.RunWithTimeout(ctx, c.runner, queryTimeout, "config", "device", "remove", container, deviceName); err != nil {
		if strings.Contains(strings.ToLower(out), "not found") {
			return nil
		}
		return fmt.Errorf("remove device %s: %w; output: %s", deviceName, err, out)
	}
	return nil
}

// PullDirectory copies a legacy container directory to a local staging
// directory. A missing source is normal for providers the project never used.
func (c *Client) PullDirectory(ctx context.Context, container, source, hostTarget string) (bool, error) {
	out, err := command.RunWithTimeout(ctx, c.runner, migrateTimeout, "file", "pull", "--recursive", container+source, hostTarget)
	if err == nil {
		return true, nil
	}
	lower := strings.ToLower(out)
	if strings.Contains(lower, "file does not exist") || strings.Contains(lower, "no such file") ||
		strings.Contains(lower, "not found") {
		return false, nil
	}
	return false, fmt.Errorf("pull %s from %s: %w; output: %s", source, container, err, out)
}

func (c *Client) Mounted(ctx context.Context, container, containerPath string) (bool, error) {
	out, err := command.RunWithTimeout(ctx, c.runner, queryTimeout, "exec", container, "--", "mountpoint", "-q", containerPath)
	if err == nil {
		return true, nil
	}
	if strings.TrimSpace(out) == "" {
		return false, nil
	}
	return false, fmt.Errorf("verify mount %s: %w; output: %s", containerPath, err, out)
}

// Busy reports whether a host-side agent invocation is currently executing in
// the project. This is the same signal the legacy shell upgrader used.
func (c *Client) Busy(ctx context.Context, container string) (bool, error) {
	pattern := regexp.QuoteMeta("lxc exec " + container + " --")
	cmd := exec.CommandContext(ctx, "pgrep", "-f", "--", pattern)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("detect active agent process: %w", err)
}

func (c *Client) EnsureBootAutostart(ctx context.Context, containerName string) error {
	lctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	cur, _ := c.runner.Run(lctx, "config", "get", containerName, "boot.autostart")
	if strings.TrimSpace(cur) == "true" {
		return nil
	}
	if out, err := c.runner.Run(lctx, "config", "set", containerName, "boot.autostart", "true"); err != nil {
		return fmt.Errorf("set boot.autostart: %w; output: %s", err, out)
	}
	return nil
}

func (c *Client) Start(ctx context.Context, containerName string) error {
	if out, err := command.RunWithTimeout(ctx, c.runner, startTimeout, "start", containerName); err != nil {
		if strings.Contains(out, "is already running") {
			return nil
		}
		return fmt.Errorf("lxc start: %w; output: %s", err, out)
	}
	return nil
}

func (c *Client) Stop(ctx context.Context, containerName string) error {
	if out, err := command.RunWithTimeout(ctx, c.runner, stopTimeout, "stop", containerName); err != nil {
		if strings.Contains(out, "not found") || strings.Contains(out, "is already stopped") {
			return nil
		}
		// Graceful shutdown needs the container's init to cooperate — a
		// workspace thrashing at its resource caps may never respond.
		// Escalate to a host-side kill so Stop always regains control.
		if fout, ferr := command.RunWithTimeout(ctx, c.runner, stopTimeout, "stop", "--force", containerName); ferr != nil {
			return fmt.Errorf("lxc stop: %w; output: %s (force follow-up: %s)", err, out, fout)
		}
	}
	return nil
}

// Restart force-restarts the container: a host-kernel kill of the process
// tree plus a fresh boot, requiring no cooperation from inside. This is the
// regain-control path for a workspace wedged at its resource limits.
func (c *Client) Restart(ctx context.Context, containerName string) error {
	if out, err := command.RunWithTimeout(ctx, c.runner, restartTimeout, "restart", "--force", containerName); err != nil {
		return fmt.Errorf("lxc restart --force: %w; output: %s", err, out)
	}
	return nil
}

func (c *Client) Delete(ctx context.Context, containerName string) error {
	if !c.runner.Available() {
		return nil
	}
	attempt := c.forceDelete(ctx, containerName)
	if attempt.err != nil {
		if strings.Contains(attempt.output, "not found") {
			return nil
		}
		if attempt.timedOut {
			missing, stateErr := c.missingAfterDeleteTimeout(ctx, containerName)
			if stateErr == nil && missing {
				return nil
			}
			if stateErr != nil {
				return fmt.Errorf("lxc delete: %w; output: %s; verify state: %v", attempt.err, attempt.output, stateErr)
			}
		}
		return fmt.Errorf("lxc delete: %w; output: %s", attempt.err, attempt.output)
	}
	return nil
}

func (c *Client) forceDelete(ctx context.Context, containerName string) deleteAttempt {
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	output, err := c.runner.Run(deleteCtx, "delete", "--force", containerName)
	timedOut := errors.Is(deleteCtx.Err(), context.DeadlineExceeded)
	cancel()
	return deleteAttempt{output: output, err: err, timedOut: timedOut}
}

// The LXD daemon can finish an asynchronous delete after the lxc client
// exceeds its deadline. Reconcile the actual instance state before reporting
// a false failure like the 0.3.0 video-editing upgrade.
func (c *Client) missingAfterDeleteTimeout(ctx context.Context, containerName string) (bool, error) {
	stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), queryTimeout)
	defer cancel()
	state, err := c.State(stateCtx, containerName)
	return state == serviceproject.ContainerStateMissing, err
}

func (c *Client) State(ctx context.Context, containerName string) (serviceproject.ContainerState, error) {
	if !c.runner.Available() {
		return serviceproject.ContainerStateUnknown, nil
	}
	out, err := command.RunWithTimeout(ctx, c.runner, queryTimeout, "info", containerName)
	if err != nil {
		if strings.Contains(out, "not found") || strings.Contains(out, "doesn't exist") {
			return serviceproject.ContainerStateMissing, nil
		}
		return serviceproject.ContainerStateUnknown, fmt.Errorf("lxc info: %w; output: %s", err, out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Status:") {
			status := strings.TrimSpace(strings.TrimPrefix(line, "Status:"))
			switch strings.ToUpper(status) {
			case "RUNNING":
				return serviceproject.ContainerStateRunning, nil
			case "STOPPED":
				return serviceproject.ContainerStateStopped, nil
			case "FROZEN":
				return serviceproject.ContainerStateFrozen, nil
			default:
				return serviceproject.ContainerStateUnknown, nil
			}
		}
	}
	return serviceproject.ContainerStateUnknown, nil
}
