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
				return runtime.InstallNPM(ctx, "fixture-package", "fixture-package@1.0.0", "fixture-binary")
			},
		},
		{
			name: "script install",
			run: func(ctx context.Context, runtime *Runtime, _ string) (string, error) {
				return runtime.InstallScript(ctx, processTreeFixture)
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

func TestRuntimeInstallNPMUpgradesPATHActivePackagePrefix(t *testing.T) {
	temporaryDirectory := t.TempDir()
	legacyPrefix := filepath.Join(temporaryDirectory, "usr-local")
	currentBin := filepath.Join(temporaryDirectory, "usr", "bin")
	legacyBin := filepath.Join(legacyPrefix, "bin")
	legacyPackage := filepath.Join(legacyPrefix, "lib", "node_modules", "@moonshot-ai", "kimi-code")
	legacyTarget := filepath.Join(legacyPackage, "dist", "main.mjs")
	for _, directory := range []string{legacyBin, filepath.Dir(legacyTarget), currentBin} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(legacyTarget, []byte("#!/bin/bash\nprintf '0.19.2\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	legacyLinkTarget := filepath.Join("..", "lib", "node_modules", "@moonshot-ai", "kimi-code", "dist", "main.mjs")
	if err := os.Symlink(legacyLinkTarget, filepath.Join(legacyBin, "kimi")); err != nil {
		t.Fatal(err)
	}

	argumentsFile := filepath.Join(temporaryDirectory, "npm-arguments")
	npmFixture := `#!/bin/bash
set -eu
printf '%s\n' "$@" > "$HOSTCLI_NPM_ARGUMENTS_FILE"
printf '%s\n' '#!/bin/bash' "printf '0.38.0\\n'" > "$HOSTCLI_ACTIVE_CLI_TARGET"
`
	if err := os.WriteFile(filepath.Join(currentBin, "npm"), []byte(npmFixture), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", legacyBin+string(os.PathListSeparator)+currentBin)
	t.Setenv("HOSTCLI_NPM_ARGUMENTS_FILE", argumentsFile)
	t.Setenv("HOSTCLI_ACTIVE_CLI_TARGET", legacyTarget)

	runtime := New()
	before, err := runtime.Version(context.Background(), "kimi", "--version")
	if err != nil || strings.TrimSpace(before) != "0.19.2" {
		t.Fatalf("version before install = %q, %v", before, err)
	}
	if _, err := runtime.InstallNPM(
		context.Background(),
		"@moonshot-ai/kimi-code",
		"@moonshot-ai/kimi-code@0.38.0",
		"kimi",
	); err != nil {
		t.Fatal(err)
	}
	after, err := runtime.Version(context.Background(), "kimi", "--version")
	if err != nil || strings.TrimSpace(after) != "0.38.0" {
		t.Fatalf("version after install = %q, %v", after, err)
	}
	arguments, err := os.ReadFile(argumentsFile)
	if err != nil {
		t.Fatal(err)
	}
	wantArguments := strings.Join([]string{
		"install",
		"-g",
		"--prefix",
		legacyPrefix,
		"@moonshot-ai/kimi-code@0.38.0",
		"--silent",
		"",
	}, "\n")
	if string(arguments) != wantArguments {
		t.Fatalf("npm arguments = %q, want %q", arguments, wantArguments)
	}
}

func TestRuntimeInstallNPMDoesNotRetargetUnownedPATHBinary(t *testing.T) {
	temporaryDirectory := t.TempDir()
	shadowBin := filepath.Join(temporaryDirectory, "shadow", "bin")
	npmBin := filepath.Join(temporaryDirectory, "npm", "bin")
	for _, directory := range []string{shadowBin, npmBin} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(shadowBin, "kimi"), []byte("#!/bin/bash\nprintf 'manual binary\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	argumentsFile := filepath.Join(temporaryDirectory, "npm-arguments")
	npmFixture := "#!/bin/bash\nprintf '%s\\n' \"$@\" > \"$HOSTCLI_NPM_ARGUMENTS_FILE\"\n"
	if err := os.WriteFile(filepath.Join(npmBin, "npm"), []byte(npmFixture), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shadowBin+string(os.PathListSeparator)+npmBin)
	t.Setenv("HOSTCLI_NPM_ARGUMENTS_FILE", argumentsFile)

	if _, err := New().InstallNPM(
		context.Background(),
		"@moonshot-ai/kimi-code",
		"@moonshot-ai/kimi-code@0.38.0",
		"kimi",
	); err != nil {
		t.Fatal(err)
	}
	arguments, err := os.ReadFile(argumentsFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(arguments), "--prefix") {
		t.Fatalf("npm was retargeted for an unowned executable: %q", arguments)
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
