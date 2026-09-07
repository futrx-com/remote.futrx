package main

import (
	"context"
	"net/http"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// What the mount controls call: two routes that report, and one that acts and
// is polled. Only one action runs per instance at a time, and its progress
// lives in this process — a plugin restart loses it, which is why the mount is
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

type operation struct {
	Action  string        `json:"action"`
	Running bool          `json:"running"`
	Result  commandResult `json:"result"`
}

func (b *backend) status(appplugin.Request) appplugin.Response {
	ctx, cancel := context.WithTimeout(context.Background(), inspectTimeout)
	defer cancel()
	service := b.command(ctx, "systemctl", "show", "s3disk.service", "--property=ActiveState,SubState,Result", "--no-pager")
	mounted := b.command(ctx, "mountpoint", "-q", "--", b.mountpoint)
	stats := b.command(ctx, "/usr/local/bin/s3disk", "status", b.mountpoint)
	return appplugin.JSON(http.StatusOK, map[string]any{"mountpoint": b.mountpoint, "mounted": mounted.Error == "", "mountCheck": mounted, "service": service, "stats": stats})
}

func (b *backend) diagnostics(appplugin.Request) appplugin.Response {
	ctx, cancel := context.WithTimeout(context.Background(), inspectTimeout)
	defer cancel()
	results := map[string]commandResult{}
	results["version"] = b.command(ctx, "/usr/local/bin/s3disk", "version")
	results["fuse"] = b.command(ctx, "ls", "-l", "/dev/fuse")
	results["journal"] = b.command(ctx, "journalctl", "-u", "s3disk.service", "-n", "60", "--no-pager", "-o", "cat")
	return appplugin.JSON(http.StatusOK, results)
}

func (b *backend) progress(appplugin.Request) appplugin.Response {
	b.mu.Lock()
	defer b.mu.Unlock()
	return appplugin.JSON(http.StatusOK, b.operation)
}

func (b *backend) start(r appplugin.Request) appplugin.Response {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.operation.Running {
		return appplugin.Errorf(http.StatusConflict, "A mount operation is already running")
	}
	action := r.Path
	b.operation = operation{Action: action, Running: true}
	go func() {
		timeout := syncTimeout
		args := []string{"/usr/local/bin/s3disk", "sync", b.mountpoint}
		if action == "restart" {
			timeout = restartTimeout
			args = []string{"systemctl", "restart", "s3disk.service"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		result := b.command(ctx, args...)
		b.mu.Lock()
		defer b.mu.Unlock()
		b.operation = operation{Action: action, Result: result}
	}()
	return appplugin.JSON(http.StatusAccepted, b.operation)
}
