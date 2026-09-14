package usersetup

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type scriptedRunner struct {
	available bool
	// scriptPresent controls the "test -x" gate; runErr is returned by bash.
	scriptPresent bool
	runErr        error
	calls         []string
	deadlines     []time.Duration
}

func (r *scriptedRunner) Available() bool { return r.available }

func (r *scriptedRunner) Run(ctx context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	if deadline, ok := ctx.Deadline(); ok {
		r.deadlines = append(r.deadlines, time.Until(deadline))
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "test -x /workspace/setup.sh") {
		if !r.scriptPresent {
			return "", errors.New("exit status 1")
		}
		return "", nil
	}
	if r.runErr != nil {
		return "apt-get update\nHit:1 ok\nE: something broke\n", r.runErr
	}
	return "ok\n", nil
}

func (r *scriptedRunner) RunStdin(ctx context.Context, _ io.Reader, args ...string) (string, error) {
	return r.Run(ctx, args...)
}

func TestEnsureSkipsMissingScript(t *testing.T) {
	runner := &scriptedRunner{available: true}
	if err := NewProvisioner(runner).Ensure(context.Background(), "project-1"); err != nil {
		t.Fatalf("Ensure = %v, want nil for missing script", err)
	}
	if len(runner.calls) != 1 || !strings.Contains(runner.calls[0], "test -x /workspace/setup.sh") {
		t.Fatalf("calls = %q, want only the executable probe", runner.calls)
	}
}

func TestEnsureRunsExecutableScript(t *testing.T) {
	runner := &scriptedRunner{available: true, scriptPresent: true}
	if err := NewProvisioner(runner).Ensure(context.Background(), "project-1"); err != nil {
		t.Fatalf("Ensure = %v, want nil", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %q, want probe + run", runner.calls)
	}
	if !strings.Contains(runner.calls[1], "exec project-1 -- bash /workspace/setup.sh") {
		t.Fatalf("run call = %q, want bash of the setup script", runner.calls[1])
	}
	if len(runner.deadlines) != 2 || runner.deadlines[1] <= runner.deadlines[0] {
		t.Fatalf("deadlines = %v, want the run budget to exceed the probe budget", runner.deadlines)
	}
}

func TestEnsureReportsTailedFailure(t *testing.T) {
	runner := &scriptedRunner{available: true, scriptPresent: true, runErr: errors.New("exit status 100")}
	err := NewProvisioner(runner).Ensure(context.Background(), "project-1")
	if err == nil || !strings.Contains(err.Error(), "/workspace/setup.sh") {
		t.Fatalf("Ensure = %v, want error naming the script", err)
	}
	if !strings.Contains(err.Error(), "something broke") {
		t.Fatalf("Ensure = %v, want output tail in the error", err)
	}
}

func TestEnsureWithoutRunner(t *testing.T) {
	runner := &scriptedRunner{}
	if err := NewProvisioner(runner).Ensure(context.Background(), "project-1"); err == nil {
		t.Fatal("Ensure = nil, want unavailability error")
	}
}
