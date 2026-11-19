package main

import (
	"context"
	"log"
	"os"

	"github.com/futrx-com/remote.futrx.com/internal/config"
	"github.com/futrx-com/remote.futrx.com/internal/stores/chatmigration"
)

// runCLICommand handles remote's non-server subcommands so main can stay a
// single top-to-bottom boot sequence instead
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
	case "storage-version":
		// A side-effect-free deployment compatibility handshake. An old binary
		// does not understand this command and must not be restored over SQLite.
		_, _ = os.Stdout.WriteString("conversations-sqlite-v1\n")
		return true
	case "storage-status":
		status, err := chatmigration.Status(ctx, cfg.DataDir)
		if err != nil {
			log.Fatalf("storage-status: %v", err)
		}
		_, _ = os.Stdout.WriteString(status + "\n")
		return true
	case "migrate-chats":
		if err := runChatMigration(ctx, cfg.DataDir, os.Stdout); err != nil {
			log.Fatalf("migrate-chats: %v", err)
		}
		return true
	case "setup-token":
		if err := runSetupToken(ctx, cfg.DataDir, cfg.BaseURL, cfg.Auth.SetupTokenTTL, os.Stdout); err != nil {
			log.Fatalf("setup-token: %v", err)
		}
		return true
	default:
		log.Fatalf("unknown command %q (supported commands: setup-token, migrate-chats, storage-version, storage-status)", command)
		return true
	}
}
