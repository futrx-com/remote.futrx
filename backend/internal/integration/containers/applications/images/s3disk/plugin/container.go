package main

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Every command this plugin runs happens inside the installed project's
// container, over the host's lxc CLI. No route supplies the command, the
// container or a path, so this is the whole of what reaches a shell.

// maxCommandOutput is what one command may return. The plugin reports command
// output to a browser, and a journal or a stat dump has no natural bound.
const maxCommandOutput = 64 << 10

type commandResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// Discard excess output while continuing to drain the process pipes.
type limitedOutput struct{ data []byte }

func (w *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := maxCommandOutput - len(w.data); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		w.data = append(w.data, p...)
	}
	return n, nil
}

func runContainer(ctx context.Context, container string, args ...string) commandResult {
	binary, err := exec.LookPath("lxc")
	if err != nil {
		binary = "/snap/bin/lxc"
	}
	commandArgs := append([]string{"exec", container, "--"}, args...)
	cmd := exec.CommandContext(ctx, binary, commandArgs...)
	cmd.WaitDelay = time.Second
	var out limitedOutput
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	result := commandResult{Output: strings.TrimSpace(string(out.data))}
	if ctx.Err() != nil {
		result.Error = "Command timed out; check mount status before retrying"
	} else if err != nil {
		result.Error = err.Error()
	}
	return result
}
