package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/repository/archives"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/repository/workspace"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/service/files"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/transport/cli"
)

// Each invocation serves one host request; the host backend bounds how many
// archives build at once.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	downloads := archives.New(config.WorkspaceDirectory, config.MaxArchiveBytes, config.DownloadRetention)
	service := files.New(config.WorkspaceDirectory, workspace.New(), downloads)
	status := cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, service)
	stop()
	os.Exit(status)
}
