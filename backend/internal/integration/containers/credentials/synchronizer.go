// Package credentials implements LXD and host-filesystem credential transfer
// mechanics for the container credential application service.
package credentials

import (
	"context"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

const (
	authPushTimeout = 30 * time.Second
	queryTimeout    = 10 * time.Second
)

// Adapter performs bidirectional credential transfer between the host's
// canonical files and their provider-defined container destinations.
type Adapter struct {
	files       fileSynchronizer
	directories directorySynchronizer
}

func NewAdapter(runner command.Runner) *Adapter {
	adapter := &Adapter{files: fileSynchronizer{runner: runner}}
	adapter.directories = directorySynchronizer{
		runner: runner,
		files:  &adapter.files,
	}
	return adapter
}

func (a *Adapter) EnsureFiles(ctx context.Context, containerName string, spec provisioning.CredentialSpec) error {
	return a.files.ensure(ctx, containerName, spec)
}

func (a *Adapter) EnsureDirectory(ctx context.Context, containerName string, spec provisioning.CredentialSpec) error {
	return a.directories.ensure(ctx, containerName, spec)
}

func (a *Adapter) SyncFilesFromContainer(ctx context.Context, containerName string, spec provisioning.CredentialSpec) error {
	return a.files.syncFromContainer(ctx, containerName, spec)
}

func (a *Adapter) SyncDirectoryFromContainer(ctx context.Context, containerName string, spec provisioning.CredentialSpec) error {
	return a.directories.syncFromContainer(ctx, containerName, spec)
}
