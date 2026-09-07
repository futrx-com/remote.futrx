// The backend runs on the Remote host; every filesystem operation runs in
// the installed project's container. No request supplies commands or targets.
package main

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin/pluginrpc"
)

func main() { pluginrpc.Serve(newBackend()) }

// containerName is the shape Remote gives a project container, and the shape
// lxc accepts. Nothing in a request reaches it — this guards against the
// plugin being installed somewhere it cannot work.
var containerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)

type backend struct {
	mux        *appplugin.Mux
	instance   appplugin.Instance
	mountpoint string
	uploadsDir string
	// asyncWriteback mirrors the mount flag of the same name. With it set, a
	// closed file is not yet on S3, which decides whether push may delete the
	// copy Remote left behind.
	asyncWriteback bool
	run            func(context.Context, string, ...string) commandResult
	mu             sync.Mutex
	operation      operation
}

func newBackend() *backend {
	b := &backend{mux: appplugin.NewMux(), run: runContainer}
	b.mux.GET("status", "Mount and service status", b.status)
	b.mux.GET("diagnostics", "Version, FUSE availability and service journal", b.diagnostics)
	b.mux.GET("operation", "Latest sync or restart progress", b.progress)
	b.mux.POST("sync", "Flush pending writes to S3", b.start)
	b.mux.POST("restart", "Restart the container mount service", b.start)
	b.mux.POST("push", "Copy finished chat uploads onto the mounted bucket", b.push)
	return b
}

func (b *backend) Describe() (appplugin.Descriptor, error) {
	return appplugin.Descriptor{Name: "s3disk", Version: "1", APIVersion: appplugin.APIVersion, Routes: b.mux.Routes()}, nil
}

func (b *backend) Init(instance appplugin.Instance) error {
	if instance.Scope != "project" || !containerName.MatchString(instance.ContainerName) {
		return fmt.Errorf("s3disk requires a project container")
	}
	mountpoint := instance.Env["S3DISK_MOUNTPOINT"]
	if mountpoint == "" {
		mountpoint = "/workspace/s3"
	}
	if !filepath.IsAbs(mountpoint) {
		return fmt.Errorf("s3disk mountpoint must be absolute")
	}
	uploadsDir, err := uploadsDirOf(instance.Env["S3DISK_UPLOADS_DIR"])
	if err != nil {
		return err
	}
	b.instance, b.mountpoint, b.uploadsDir = instance, filepath.Clean(mountpoint), uploadsDir
	b.asyncWriteback = hasMountFlag(instance.Env["S3DISK_MOUNT_ARGS"], "--async-writeback")
	return nil
}

// hasMountFlag reports whether the operator passed a flag through
// S3DISK_MOUNT_ARGS. It matches the flag as a whole word so "--async-writeback"
// is not found inside "--no-async-writeback".
func hasMountFlag(args, flag string) bool {
	for _, field := range strings.Fields(args) {
		if field == flag || strings.HasPrefix(field, flag+"=") {
			return true
		}
	}
	return false
}

func (b *backend) Handle(r appplugin.Request) (appplugin.Response, error) { return b.mux.Serve(r), nil }

func (b *backend) command(ctx context.Context, args ...string) commandResult {
	result := b.run(ctx, b.instance.ContainerName, args...)
	// Also redact credentials if a failing external command happens to echo them.
	for key, value := range b.instance.Env {
		if value != "" && (strings.Contains(key, "SECRET") || strings.Contains(key, "KEY") || strings.Contains(key, "TOKEN")) {
			result.Output = strings.ReplaceAll(result.Output, value, "[redacted]")
			result.Error = strings.ReplaceAll(result.Error, value, "[redacted]")
		}
	}
	return result
}
