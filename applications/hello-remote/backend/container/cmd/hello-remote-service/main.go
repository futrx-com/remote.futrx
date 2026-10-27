package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

var version = "development"

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string, stdout io.Writer) error {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(stdout, version)
		return nil
	}
	if len(args) == 0 {
		return errors.New("usage: hello-remote-service serve|health [options]")
	}

	switch args[0] {
	case "serve":
		port, err := parsePort("serve", "TCP port to listen on", args[1:])
		if err != nil {
			return err
		}
		return serve(port, getenv)
	case "health":
		port, err := parsePort("health", "TCP port to probe", args[1:])
		if err != nil {
			return err
		}
		return health(port)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func parsePort(command, description string, args []string) (int, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	port := flags.Int("port", 0, description)
	if err := flags.Parse(args); err != nil {
		return 0, err
	}
	if *port < 1 || *port > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535")
	}
	return *port, nil
}
