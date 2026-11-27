//go:build linux

package containerio

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTheBridgeIsOneFixedCommand(t *testing.T) {
	cmd := BridgeCommand("project-demo")()
	want := []string{"exec", "project-demo", "--", "/usr/local/bin/remote-terminal", "bridge"}
	if !slices.Equal(cmd.Args[1:], want) {
		t.Fatalf("bridge = %q, want lxc %q", cmd.Args, want)
	}
}

func TestABridgeThatExitsFailsItsRequestsAndIsReplaced(t *testing.T) {
	starts := 0
	bridge := NewBridge(func() *exec.Cmd {
		starts++
		return exec.Command("/bin/sh", "-c", "read line; echo 'Error: Instance is not running' >&2; exit 1")
	})
	defer bridge.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for attempt := 1; attempt <= 2; attempt++ {
		_, err := bridge.Do(ctx, Request{Method: "GET", Path: "/output"})
		if !errors.Is(err, errBridgeExited) || !strings.Contains(err.Error(), "Instance is not running") {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	if starts != 2 {
		t.Fatalf("bridge started %d times, want once per failed call", starts)
	}
}
