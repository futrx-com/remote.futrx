package kimi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func fakeCLI(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kimi"), []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
func TestRunReportsServerStartupDiagnostics(t *testing.T) {
	fakeCLI(t, `printf '%s\n' 'API request failed: 307 Temporary Redirect' >&2
exit 1`)
	var failures, completions int
	err := (&Provider{}).Run(context.Background(), agent.RunRequest{Cwd: t.TempDir()}, func(ev agent.Event) {
		if ev.Type == agent.EventRunCompleted {
			completions++
		}
		if ev.Type == agent.EventRunFailed {
			failures++
			if !strings.Contains(ev.Message, "307 Temporary Redirect") {
				t.Error(ev.Message)
			}
		}
	})
	if !errors.Is(err, agent.ErrRunFailed) || failures != 1 || completions != 0 {
		t.Fatalf("err=%v failures=%d completions=%d", err, failures, completions)
	}
}
func TestRunRejectsUnknownModeBeforeLaunching(t *testing.T) {
	err := (&Provider{}).Run(context.Background(), agent.RunRequest{Mode: "unknown"}, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported Kimi mode") {
		t.Fatalf("err=%v", err)
	}
}
