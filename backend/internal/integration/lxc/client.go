package lxc

// Thin wrapper around the `lxc` CLI. This package is intentionally policy-free:
// it knows how to invoke the binary and capture its output, nothing else.
// Higher-level orchestration (container lifecycle, auth bundles, software
// provisioning) lives in internal/integration/containers.

import (
	"context"
	"io"
	"os/exec"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

// Client is the value used to talk to the local LXD CLI. Construct with New().
type Client struct {
	bin string
}

// New returns a Client that uses the `lxc` binary on PATH.
func New() *Client {
	return &Client{bin: "lxc"}
}

// Available reports whether the lxc binary is reachable on PATH.
func (c *Client) Available() bool {
	_, err := exec.LookPath(c.bin)
	return err == nil
}

// Run invokes `lxc <args>` and returns the combined stdout+stderr. Errors are
// returned verbatim from os/exec; callers inspect the output to disambiguate
// (e.g. distinguish "not found" from other failures).
func (c *Client) Run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.bin, args...)
	return command.CaptureOutput(ctx, cmd)
}

// RunStdin is Run with stdin attached. Used by callers that pipe content into
// `lxc exec ... -- tee <path>` or similar.
func (c *Client) RunStdin(ctx context.Context, stdin io.Reader, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Stdin = stdin
	return command.CaptureOutput(ctx, cmd)
}
