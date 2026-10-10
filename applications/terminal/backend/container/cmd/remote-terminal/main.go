//go:build linux

// remote-terminal serves shell sessions of a project container to the Remote
// web gateway. Sessions outlive the WebSocket that shows them, so a dropped
// connection reattaches to the same shell instead of starting a new one.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] != "serve" {
		return errors.New("usage: remote-terminal serve --port <port>")
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	port := flags.Int("port", 0, "TCP port to listen on")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *port < 1 || *port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	return serve(*port)
}
