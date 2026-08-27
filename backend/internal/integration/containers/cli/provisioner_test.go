package cli

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

type recordingRunner struct {
	calls     []string
	deadlines []time.Duration
}

func (r *recordingRunner) Available() bool { return true }

func (r *recordingRunner) Run(ctx context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	if deadline, ok := ctx.Deadline(); ok {
		r.deadlines = append(r.deadlines, time.Until(deadline))
	} else {
		r.deadlines = append(r.deadlines, 0)
	}
	if len(args) > 4 && args[3] == "pgrep" {
		return "123\n", nil
	}
	return "agent 1.2.3\n", nil
}

func (r *recordingRunner) RunStdin(ctx context.Context, _ io.Reader, args ...string) (string, error) {
	return r.Run(ctx, args...)
}

func TestClientTranslatesCLIOperationsToLXDArguments(t *testing.T) {
	runner := &recordingRunner{}
	client := NewClient(runner)
	installCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if !client.Available() {
		t.Fatal("Available = false, want runner availability")
	}

	version, err := client.Version(context.Background(), "c1", "agent", "version")
	if err != nil || version != "agent 1.2.3\n" {
		t.Fatalf("Version = %q, %v", version, err)
	}
	if !client.CommandExists(context.Background(), "c1", "npm") {
		t.Fatal("CommandExists = false, want true")
	}
	if !client.InstallRunning(context.Background(), "c1", "@vendor/agent") {
		t.Fatal("InstallRunning = false, want true")
	}
	if _, err := client.InstallNPM(installCtx, "c1", "@vendor/agent@1.2.3"); err != nil {
		t.Fatalf("InstallNPM: %v", err)
	}
	if _, err := client.Repair(installCtx, "c1", "install-script"); err != nil {
		t.Fatalf("Repair: %v", err)
	}

	wantCalls := []string{
		"exec c1 -- agent version",
		"exec c1 -- which npm",
		"exec c1 -- pgrep -f npm install.*@vendor/agent",
		"exec c1 -- npm install -g @vendor/agent@1.2.3 --silent",
		"exec c1 -- bash -c install-script",
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
	}
	for index, remaining := range runner.deadlines[:3] {
		if remaining <= queryTimeout-time.Second || remaining > queryTimeout {
			t.Fatalf("query deadline[%d] = %s, want approximately %s", index, remaining, queryTimeout)
		}
	}
	for index, remaining := range runner.deadlines[3:] {
		if remaining <= 3*time.Minute-time.Second || remaining > 3*time.Minute {
			t.Fatalf("mutation deadline[%d] = %s, want caller deadline", index, remaining)
		}
	}
}
