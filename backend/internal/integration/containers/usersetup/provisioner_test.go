package usersetup

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

// localRunner checks the LXD execution contract and runs the actual guest
// wrapper against isolated workspace/rootfs directories. No LXD is required.
type localRunner struct {
	t                 *testing.T
	workspace, rootfs string
	unavailable       bool
	err               error
}

func newLocalRunner(t *testing.T) *localRunner {
	t.Helper()
	return &localRunner{t: t, workspace: t.TempDir(), rootfs: t.TempDir()}
}

func (r *localRunner) Available() bool { return !r.unavailable }

func (r *localRunner) Run(ctx context.Context, args ...string) (string, error) {
	if r.err != nil {
		return "transport failed", r.err
	}
	if !slices.Equal(args[:8], []string{"exec", "project-1", "--cwd", "/workspace", "--user", "0", "--group", "0"}) {
		r.t.Fatalf("unexpected execution context: %q", args[:8])
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > setupRunTimeout {
		r.t.Fatal("missing setup deadline")
	}
	env := os.Environ()
	i := 8
	for args[i] == "--env" {
		env = append(env, args[i+1])
		i += 2
	}
	if args[i] != "--" {
		r.t.Fatalf("unexpected arguments: %q", args[i:])
	}
	guest := slices.Clone(args[i+1:])
	if !slices.Equal(guest[:3], []string{"timeout", "--kill-after=5s", "15m"}) {
		r.t.Fatal("missing guest timeout")
	}
	guest[len(guest)-2] = filepath.Join(r.workspace, "setup.sh")
	guest[len(guest)-1] = filepath.Join(r.rootfs, "state", "user-setup.sha256")
	cmd := exec.CommandContext(ctx, guest[0], guest[1:]...)
	cmd.Dir, cmd.Env = r.workspace, env
	return command.CaptureOutput(ctx, cmd)
}

func (r *localRunner) RunStdin(context.Context, io.Reader, ...string) (string, error) {
	panic("unexpected stdin execution")
}

func (r *localRunner) script(text string, mode os.FileMode) {
	r.t.Helper()
	path := filepath.Join(r.workspace, "setup.sh")
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		r.t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		r.t.Fatal(err)
	}
}

func TestSetupSkipsAbsentAndNonExecutableScripts(t *testing.T) {
	runner := newLocalRunner(t)
	setup := NewProvisioner(runner)
	if err := setup.Ensure(context.Background(), "project-1", nil); err != nil {
		t.Fatal(err)
	}
	runner.script("exit 99\n", 0600)
	if err := setup.Ensure(context.Background(), "project-1", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runner.rootfs, "state")); !os.IsNotExist(err) {
		t.Fatal("skipped setup created state")
	}
}

func TestSetupUsesWorkspaceAndSecretsAndSkipsCompletedRevision(t *testing.T) {
	runner := newLocalRunner(t)
	runner.script("set -eu\ncat requirements.txt > result\nprintf '%s' \"$SETUP_TEST_SECRET\" >> result\necho run >> runs\n", 0700)
	if err := os.WriteFile(filepath.Join(runner.workspace, "requirements.txt"), []byte("relative file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"SETUP_TEST_SECRET": "dummy-token\nsecond-line"}
	setup := NewProvisioner(runner)
	for range 2 {
		if err := setup.Ensure(context.Background(), "project-1", env); err != nil {
			t.Fatal(err)
		}
	}
	result, _ := os.ReadFile(filepath.Join(runner.workspace, "result"))
	if string(result) != "relative file\n"+env["SETUP_TEST_SECRET"] {
		t.Fatalf("result = %q", result)
	}
	runs, _ := os.ReadFile(filepath.Join(runner.workspace, "runs"))
	if string(runs) != "run\n" {
		t.Fatalf("successful setup reran: %q", runs)
	}
	// A new rootfs loses the marker but retains the workspace.
	runner.rootfs = t.TempDir()
	if err := setup.Ensure(context.Background(), "project-1", env); err != nil {
		t.Fatal(err)
	}
	runs, _ = os.ReadFile(filepath.Join(runner.workspace, "runs"))
	if string(runs) != "run\nrun\n" {
		t.Fatalf("replacement did not restore tools: %q", runs)
	}
}

func TestSetupFailureRetriesSameScriptAndChangedScriptReplays(t *testing.T) {
	runner := newLocalRunner(t)
	runner.script("set -eu\ntest -f ready\necho restored >> runs\n", 0700)
	setup := NewProvisioner(runner)
	if err := setup.Ensure(context.Background(), "project-1", nil); err == nil {
		t.Fatal("failure was hidden")
	}
	if _, err := os.Stat(filepath.Join(runner.rootfs, "state", "user-setup.sha256")); !os.IsNotExist(err) {
		t.Fatal("failure recorded success")
	}
	if err := os.WriteFile(filepath.Join(runner.workspace, "ready"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := setup.Ensure(context.Background(), "project-1", nil); err != nil {
		t.Fatal(err)
	}
	runner.script("echo updated >> runs\n", 0700)
	if err := setup.Ensure(context.Background(), "project-1", nil); err != nil {
		t.Fatal(err)
	}
	runs, _ := os.ReadFile(filepath.Join(runner.workspace, "runs"))
	if string(runs) != "restored\nupdated\n" {
		t.Fatalf("retry/revision replay failed: %q", runs)
	}
}

func TestSetupConcurrentAttemptsExecuteOnce(t *testing.T) {
	runner := newLocalRunner(t)
	runner.script("sleep 0.05\necho run >> runs\n", 0700)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := NewProvisioner(runner).Ensure(context.Background(), "project-1", nil); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	runs, _ := os.ReadFile(filepath.Join(runner.workspace, "runs"))
	if string(runs) != "run\n" {
		t.Fatalf("concurrent setup ran more than once: %q", runs)
	}
}

func TestSetupStopsAtUnhandledCommandFailure(t *testing.T) {
	runner := newLocalRunner(t)
	runner.script("false\necho incorrect-success > result\n", 0700)
	if err := NewProvisioner(runner).Ensure(context.Background(), "project-1", nil); err == nil {
		t.Fatal("earlier command failure was hidden by a later successful command")
	}
	if _, err := os.Stat(filepath.Join(runner.workspace, "result")); !os.IsNotExist(err) {
		t.Fatal("script continued after failure")
	}
}

func TestSetupReportsTransportFailuresAndCancellation(t *testing.T) {
	for _, cause := range []error{context.DeadlineExceeded, context.Canceled, errors.New("LXD unavailable")} {
		runner := newLocalRunner(t)
		runner.err = cause
		if err := NewProvisioner(runner).Ensure(context.Background(), "project-1", nil); !errors.Is(err, cause) {
			t.Fatalf("error = %v, want %v", err, cause)
		}
	}
	runner := newLocalRunner(t)
	runner.unavailable = true
	if err := NewProvisioner(runner).Ensure(context.Background(), "project-1", nil); !errors.Is(err, command.ErrUnavailable) {
		t.Fatalf("unavailable error = %v", err)
	}
}

func TestSetupFailureOutputIsBoundedAndSecretsAreRedacted(t *testing.T) {
	runner := newLocalRunner(t)
	runner.script("printf '%s' \"$SETUP_TEST_SECRET\"\nexit 1\n", 0700)
	env := map[string]string{"SETUP_TEST_SECRET": "dummy-private-token"}
	err := NewProvisioner(runner).Ensure(context.Background(), "project-1", env)
	if err == nil || strings.Contains(err.Error(), env["SETUP_TEST_SECRET"]) || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("unsafe failure: %v", err)
	}
	runner.script("head -c 1048576 /dev/zero | tr '\\000' x\nprintf LAST_ERROR\nexit 1\n", 0700)
	err = NewProvisioner(runner).Ensure(context.Background(), "project-1", nil)
	if err == nil || len(err.Error()) > 4500 || !strings.Contains(err.Error(), "LAST_ERROR") {
		t.Fatalf("unbounded/missing failure tail: %v", err)
	}
	if got := failureOutput(strings.Repeat("x", setupOutputLimit), env); got != "setup output exceeded the capture limit" {
		t.Fatalf("truncated secret handling = %q", got)
	}
}
