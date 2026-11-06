//go:build linux

// remote-terminal keeps the project's shells inside its container.
//
//	remote-terminal serve   runs as remote-terminal.service and owns every
//	                        shell, answering on a root-only Unix socket
//	remote-terminal bridge  relays Terminal's host backend requests from stdin
//	                        to that socket; the host runs it through `lxc exec`
//
// Nothing listens on a TCP port. Shells belong to the service, so they outlive
// the bridge, the host backend and Remote itself restarting.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"futrx.local/catalog/applications/terminal/backend/container/internal/bridge"
	"futrx.local/catalog/applications/terminal/backend/container/internal/server"
	"futrx.local/catalog/applications/terminal/backend/container/internal/sessions"
	"futrx.local/catalog/applications/terminal/backend/container/internal/shell"
)

const (
	defaultSocket      = "/run/remote-terminal/terminal.sock"
	workspaceDirectory = "/workspace"

	// A shell nobody has read for this long is ended.
	detachedTTL  = 10 * time.Minute
	maxSessions  = 64
	replayBytes  = 256 << 10
	pollWait     = 20 * time.Second
	reapInterval = 30 * time.Second
)

const usage = "usage: remote-terminal serve|bridge [--socket <path>]"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	socket := flags.String("socket", defaultSocket, "service socket")
	workspace := flags.String("workspace", workspaceDirectory, "directory shells start in and stay under")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "serve":
		listener, err := server.Listen(*socket)
		if err != nil {
			return err
		}
		manager := sessions.NewManager(sessions.Config{
			Command:      shell.Command(*workspace, os.Getenv),
			DetachedTTL:  detachedTTL,
			MaxSessions:  maxSessions,
			ReplayBytes:  replayBytes,
			PollWait:     pollWait,
			ReapInterval: reapInterval,
		})
		defer manager.Close()
		return server.Serve(listener, manager)
	case "bridge":
		return bridge.Run(stdin, stdout, *socket)
	}
	return errors.New(usage)
}
