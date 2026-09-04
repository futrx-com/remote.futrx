//go:build linux

package hostcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const processTreeFixture = `#!/bin/bash
set -eu
printf 'preserved combined output\n'
trap '' TERM
(
    trap '' TERM
    printf '%s\n' "$BASHPID" > "$HOSTCLI_CHILD_PID_FILE"
    while :; do sleep 1; done
) &
wait
`

func TestRuntimeCancellationTerminatesCompleteProcessGroup(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *Runtime, string) (string, error)
	}{
		{
			name: "version",
			run: func(ctx context.Context, runtime *Runtime, fixture string) (string, error) {
				return runtime.Version(ctx, fixture)
			},
		},
		{
			name: "npm install",
			run: func(ctx context.Context, runtime *Runtime, _ string) (string, error) {
				return runtime.InstallNPM(ctx, "/managed/host-clis", "fixture-package@1.0.0")
			},
		},
		{
			name: "script install",
			run: func(ctx context.Context, runtime *Runtime, _ string) (string, error) {
				return runtime.InstallScript(ctx, processTreeFixture, "/managed/host-clis/bin/fixture-command")
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			temporaryDirectory := t.TempDir()
			fixture := filepath.Join(temporaryDirectory, "fixture-command")
			if err := os.WriteFile(fixture, []byte(processTreeFixture), 0o700); err != nil {
				t.Fatal(err)
			}
			// InstallNPM resolves `npm` through PATH; point it at the same
			// process-tree fixture without affecting the other command cases.
			if err := os.Symlink(fixture, filepath.Join(temporaryDirectory, "npm")); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
			pidFile := filepath.Join(temporaryDirectory, "child.pid")
			t.Setenv("HOSTCLI_CHILD_PID_FILE", pidFile)

			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan commandResult, 1)
			go func() {
				output, err := testCase.run(ctx, New(), fixture)
				result <- commandResult{output: output, err: err}
			}()

			childPID := waitForChildPID(t, pidFile)
			canceledAt := time.Now()
			cancel()

			var command commandResult
			select {
			case command = <-result:
			case <-time.After(2 * time.Second):
				t.Fatal("canceled host command did not return promptly")
			}
			if elapsed := time.Since(canceledAt); elapsed > time.Second {
				t.Fatalf("canceled host command returned after %s", elapsed)
			}
			if !errors.Is(command.err, context.Canceled) {
				t.Fatalf("error = %v, want context cancellation", command.err)
			}
			if !strings.Contains(command.output, "preserved combined output") {
				t.Fatalf("combined output = %q", command.output)
			}
			waitForProcessExit(t, childPID)
		})
	}
}

func TestRuntimeInstallersReceiveCanonicalManagedLocation(t *testing.T) {
	temporaryDirectory := t.TempDir()
	npmFixture := filepath.Join(temporaryDirectory, "npm")
	if err := os.WriteFile(npmFixture, []byte("#!/bin/bash\nprintf '%s\\n' \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	runtime := New()
	npmOutput, err := runtime.InstallNPM(context.Background(), "/managed/host-clis", "fixture-package@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	wantNPMOutput := strings.Join([]string{
		"install",
		"-g",
		"--prefix",
		"/managed/host-clis",
		"fixture-package@1.0.0",
		"--silent",
		"",
	}, "\n")
	if npmOutput != wantNPMOutput {
		t.Fatalf("npm output = %q, want %q", npmOutput, wantNPMOutput)
	}

	managedExecutable := "/managed/host-clis/bin/fixture-command"
	scriptOutput, err := runtime.InstallScript(
		context.Background(),
		`printf '%s\n' "$FUTRX_HOST_CLI_INSTALL_PATH"`,
		managedExecutable,
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(scriptOutput) != managedExecutable {
		t.Fatalf("script install path = %q, want %q", scriptOutput, managedExecutable)
	}
}

type commandResult struct {
	output string
	err    error
}

func waitForChildPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(contents)))
			if parseErr != nil {
				t.Fatalf("parse child PID %q: %v", contents, parseErr)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read child PID: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("child process did not start")
	return 0
}

func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil {
			t.Fatalf("query child process %d: %v", pid, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(fmt.Sprintf("child process %d survived process-group cancellation", pid))
}
