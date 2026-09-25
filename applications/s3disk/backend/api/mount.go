package api

import (
	"context"
	"net/http"
	"time"

	appLifecycle "futrx.local/catalog/applications/s3disk/backend/lifecycle"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// What the mount controls call: two routes that report, and one that acts and
// is polled. Only one action runs per instance at a time, and its progress
// lives in this process — a backend restart loses it, which is why the mount is
// inspected rather than the operation retried.
const (
	// inspectTimeout bounds a report. These run while somebody waits on a
	// popup, so they fail fast rather than hang.
	inspectTimeout = 8 * time.Second
	// syncTimeout bounds a flush of everything still pending. It is generous
	// because the work is an upload of unknown size.
	syncTimeout = 61 * time.Minute
	// restartTimeout covers a stop that flushes before it detaches.
	restartTimeout = 6 * time.Minute
)

func (b *backend) status(applications.Request) applications.Response {
	ctx, cancel := context.WithTimeout(context.Background(), inspectTimeout)
	defer cancel()
	service := b.command(ctx, "systemctl", "show", b.target.service, "--property=ActiveState,SubState,Result", "--no-pager")
	mounted := b.command(ctx, "mountpoint", "-q", "--", b.target.mountpoint)
	stats := b.command(ctx, b.target.binary, "status", b.target.mountpoint)
	return applications.JSON(http.StatusOK, map[string]any{"mountpoint": b.target.mountpoint, "mounted": mounted.Error == "", "mountCheck": mounted, "service": service, "stats": stats})
}

func (b *backend) diagnostics(applications.Request) applications.Response {
	ctx, cancel := context.WithTimeout(context.Background(), inspectTimeout)
	defer cancel()
	results := map[string]commandResult{}
	results["version"] = b.command(ctx, b.target.binary, "version")
	results["fuse"] = b.command(ctx, "ls", "-l", "/dev/fuse")
	results["journal"] = b.command(ctx, "journalctl", "-u", b.target.service, "-n", "60", "--no-pager", "-o", "cat")
	return applications.JSON(http.StatusOK, results)
}

func (b *backend) progress(applications.Request) applications.Response {
	return applications.JSON(http.StatusOK, b.operations.Snapshot())
}

func (b *backend) start(r applications.Request) applications.Response {
	action := r.Path
	started, ok := b.operations.Start(action, func() appLifecycle.Result {
		timeout := syncTimeout
		args := []string{b.target.binary, "sync", b.target.mountpoint}
		if action == "restart" {
			timeout = restartTimeout
			args = []string{"systemctl", "restart", b.target.service}
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		result := b.command(ctx, args...)
		return appLifecycle.Result{Output: result.Output, Error: result.Error}
	})
	if !ok {
		return applications.Errorf(http.StatusConflict, "A mount operation is already running")
	}
	return applications.JSON(http.StatusAccepted, started)
}
