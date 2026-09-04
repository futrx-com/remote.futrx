package main

import (
	"context"
	"log"
	"os"

	"github.com/futrx-com/remote.futrx.com/internal/config"
)

// runCLICommand handles remote's non-server subcommands (currently just
// setup-token) so main can stay a single top-to-bottom boot sequence instead
// of branching into command dispatch itself.
//
// It reports whether args named a command at all; every branch that finds one
// either returns after running it or calls log.Fatalf, so main always returns
// immediately once this reports true.
func runCLICommand(ctx context.Context, cfg config.Config, args []string) bool {
	if len(args) <= 1 {
		return false
	}
	switch command := args[1]; command {
	case "setup-token":
		if err := runSetupToken(ctx, cfg.DataDir, cfg.BaseURL, cfg.Auth.SetupTokenTTL, os.Stdout); err != nil {
			log.Fatalf("setup-token: %v", err)
		}
		return true
	default:
		log.Fatalf("unknown command %q (supported commands: setup-token)", command)
		return true
	}
}
