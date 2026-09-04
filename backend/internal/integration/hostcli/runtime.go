// Package hostcli translates host agent CLI operations into local processes.
// Installation policy remains in the service layer and provider profiles.
package hostcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const processTerminationGrace = 250 * time.Millisecond

type Runtime struct{}

func New() *Runtime {
	return &Runtime{}
}

func (*Runtime) CommandExists(executablePath string) bool {
	_, err := exec.LookPath(executablePath)
	return err == nil
}

func (*Runtime) ExecutablePath(binary string) string {
	executablePath, err := exec.LookPath(binary)
	if err != nil {
		return ""
	}
	absolutePath, err := filepath.Abs(executablePath)
	if err == nil {
		executablePath = absolutePath
	}
	return filepath.Clean(executablePath)
}

func (*Runtime) Version(ctx context.Context, executablePath string, arguments ...string) (string, error) {
	return runInProcessGroup(ctx, executablePath, nil, arguments...)
}

func (*Runtime) InstallNPM(ctx context.Context, managedPrefix, npmPackage string) (string, error) {
	return runInProcessGroup(ctx, "npm", nil, "install", "-g", "--prefix", managedPrefix, npmPackage, "--silent")
}

func (*Runtime) InstallScript(ctx context.Context, script, managedExecutable string) (string, error) {
	environment := append(os.Environ(), "FUTRX_HOST_CLI_INSTALL_PATH="+managedExecutable)
	return runInProcessGroup(ctx, "/bin/bash", environment, "-c", script)
}

// runInProcessGroup isolates every provider-owned command from the updater's
// process group. Canceling the context first gives the full command tree a
// brief opportunity to clean up, then kills any descendants that ignored
// SIGTERM. Capturing both streams in one buffer preserves CombinedOutput's
// ordering and ensures Wait does not return while a descendant still owns an
// output descriptor.
func runInProcessGroup(ctx context.Context, name string, environment []string, arguments ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	command := exec.Command(name, arguments...)
	if environment != nil {
		command.Env = environment
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		return output.String(), err
	}

	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()

	select {
	case err := <-done:
		return output.String(), err
	case <-ctx.Done():
	}

	processGroup := -command.Process.Pid
	termErr := syscall.Kill(processGroup, syscall.SIGTERM)
	if termErr != nil && !errors.Is(termErr, syscall.ESRCH) {
		// A same-user process group should always be signalable. Fall back to
		// killing the leader so a surprising platform error cannot leave the
		// updater blocked forever.
		_ = command.Process.Kill()
	}

	timer := time.NewTimer(processTerminationGrace)
	defer timer.Stop()
	select {
	case waitErr := <-done:
		// A descendant may close its output descriptors and survive its leader.
		// Kill any remaining member immediately before the process-group ID can
		// be reused.
		_ = syscall.Kill(processGroup, syscall.SIGKILL)
		return output.String(), canceledCommandError(ctx.Err(), waitErr)
	case <-timer.C:
		_ = syscall.Kill(processGroup, syscall.SIGKILL)
		waitErr := <-done
		return output.String(), canceledCommandError(ctx.Err(), waitErr)
	}
}

func canceledCommandError(contextErr, processErr error) error {
	if contextErr == nil {
		return processErr
	}
	if processErr == nil {
		return contextErr
	}
	return fmt.Errorf("%w (process: %v)", contextErr, processErr)
}
