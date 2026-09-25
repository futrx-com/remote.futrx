// The backend runs on the Remote host; every filesystem operation runs in
// the installed project's container. No request supplies commands or targets.
package api

import (
	"context"
	"strings"

	appLifecycle "futrx.local/catalog/applications/s3disk/backend/lifecycle"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

func New(operations appLifecycle.MountOperations, pushes appLifecycle.PushEvents) applications.Backend {
	return newBackend(operations, pushes)
}

type backend struct {
	router      *applications.Router
	target      backendTarget
	run         func(context.Context, string, ...string) commandResult
	operations  appLifecycle.MountOperations
	pushes      appLifecycle.PushEvents
	attachments *attachmentCopier
}

func newBackend(operations appLifecycle.MountOperations, pushes appLifecycle.PushEvents) *backend {
	b := &backend{router: applications.NewRouter(), run: runContainer, operations: operations, pushes: pushes}
	b.router.GET("status", "Mount and service status", b.status)
	b.router.GET("diagnostics", "Version, FUSE availability and service journal", b.diagnostics)
	b.router.GET("operation", "Latest sync or restart progress", b.progress)
	b.router.POST("sync", "Flush pending writes to S3", b.start)
	b.router.POST("restart", "Restart the container mount service", b.start)
	b.router.POST("push", "Copy finished chat uploads onto the mounted bucket", b.push)
	return b
}

// REQUIRED — the host calls Describe once during the plugin handshake.
// APIVersion must be applications.APIVersion or Remote refuses the backend.
func (b *backend) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{APIVersion: applications.APIVersion, Routes: b.router.Routes()}, nil
}

// REQUIRED — the host calls Init once before the first request.
func (b *backend) Init(instance applications.Instance) error {
	target, err := newBackendTarget(instance)
	if err != nil {
		return err
	}
	b.target = target
	b.attachments = newAttachmentCopier(target.mountpoint, target.uploadsDir, target.asyncWriteback,
		func(ctx context.Context, args ...string) (string, string) {
			result := b.command(ctx, args...)
			return result.Output, result.Error
		})
	return nil
}

// REQUIRED — the host calls Handle for each backend request, potentially
// concurrently. Router is optional; it is s3disk's route dispatcher.
func (b *backend) Handle(r applications.Request) (applications.Response, error) {
	return b.router.Serve(r), nil
}

func (b *backend) command(ctx context.Context, args ...string) commandResult {
	result := b.run(ctx, b.target.instance.ContainerName, args...)
	// Also redact credentials if a failing external command happens to echo them.
	for key, value := range b.target.instance.Env {
		if value != "" && (strings.Contains(key, "SECRET") || strings.Contains(key, "KEY") || strings.Contains(key, "TOKEN")) {
			result.Output = strings.ReplaceAll(result.Output, value, "[redacted]")
			result.Error = strings.ReplaceAll(result.Error, value, "[redacted]")
		}
	}
	return result
}
