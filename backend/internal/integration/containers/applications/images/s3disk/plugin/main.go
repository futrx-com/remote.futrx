// The backend runs on the Remote host; every filesystem operation runs in
// the installed project's container. No request supplies commands or targets.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin/pluginrpc"
)

func main() { pluginrpc.Serve(newBackend()) }

type commandResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}
type operation struct {
	Action  string        `json:"action"`
	Running bool          `json:"running"`
	Result  commandResult `json:"result"`
}
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

// uploadsSource is where Remote puts a project chat's attachments. The server
// writes them from the host, into the directory the container sees as
// /workspace, so they arrive beside the mount rather than inside it — copying
// them across is the whole point of the push route, and it must run in the
// container because that is where the FUSE mount exists.
const uploadsSource = "/workspace/.uploads"

// defaultUploadsDir is where inside the bucket pushed attachments land.
const defaultUploadsDir = "uploads"

const (
	// maxPushNames bounds one request. A chat sends a handful of attachments;
	// anything larger is a caller that should be paginating.
	maxPushNames = 32
	// pushTimeout bounds the whole batch. The mount writes back synchronously
	// unless --async-writeback is set, so a copy takes as long as the upload
	// to S3 does; image.json widens the transport timeout to cover this.
	pushTimeout = 90 * time.Second
	// maxFailureText keeps one file's error readable in a batch result.
	maxFailureText = 500
)

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
	if instance.Scope != "project" || !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`).MatchString(instance.ContainerName) {
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

// uploadsDirOf resolves where inside the bucket pushed attachments land. It is
// an operator setting, not a caller's, but it still ends up in a command line:
// "." and "/" mean the mount root, and anything else has to be a plain
// relative path so it cannot climb out of the mount or look like a flag.
func uploadsDirOf(configured string) (string, error) {
	configured = strings.Trim(strings.TrimSpace(configured), "/")
	switch configured {
	case "":
		return defaultUploadsDir, nil
	case ".":
		return "", nil
	}
	for _, segment := range strings.Split(configured, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.HasPrefix(segment, "-") {
			return "", fmt.Errorf("S3DISK_UPLOADS_DIR must be a plain relative path")
		}
	}
	return configured, nil
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
func (b *backend) status(appplugin.Request) appplugin.Response {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	service := b.command(ctx, "systemctl", "show", "s3disk.service", "--property=ActiveState,SubState,Result", "--no-pager")
	mounted := b.command(ctx, "mountpoint", "-q", "--", b.mountpoint)
	stats := b.command(ctx, "/usr/local/bin/s3disk", "status", b.mountpoint)
	return appplugin.JSON(http.StatusOK, map[string]any{"mountpoint": b.mountpoint, "mounted": mounted.Error == "", "mountCheck": mounted, "service": service, "stats": stats})
}
func (b *backend) diagnostics(appplugin.Request) appplugin.Response {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
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
		timeout := 61 * time.Minute
		args := []string{"/usr/local/bin/s3disk", "sync", b.mountpoint}
		if action == "restart" {
			timeout = 6 * time.Minute
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

// pushRequest is the only route that takes anything from the caller, and it
// takes file names alone: both directories involved belong to this plugin, so
// a name is all there is left to say.
type pushRequest struct {
	Names []string `json:"names"`
}

type pushResult struct {
	Name string `json:"name"`
	// Path is where the file now is, so the caller can point at it rather
	// than rebuild it from the directory and name.
	Path string `json:"path"`
	// Stored reports the file is on the mount, whether this call put it there
	// or found it already present.
	Stored bool `json:"stored"`
	// Removed reports the copy in .uploads is gone. It is only ever set after
	// Stored, so a caller can treat it as "the bucket is now the only copy".
	Removed bool   `json:"removed"`
	Skipped bool   `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (b *backend) push(r appplugin.Request) appplugin.Response {
	var request pushRequest
	if err := json.Unmarshal(r.Body, &request); err != nil {
		return appplugin.Errorf(http.StatusBadRequest, "Malformed request body")
	}
	if len(request.Names) == 0 {
		return appplugin.Errorf(http.StatusBadRequest, "No file names were given")
	}
	if len(request.Names) > maxPushNames {
		return appplugin.Errorf(
			http.StatusBadRequest, "At most %d files can be pushed at once", maxPushNames)
	}

	ctx, cancel := context.WithTimeout(context.Background(), pushTimeout)
	defer cancel()

	// Copying into an unmounted mountpoint would write to the plain directory
	// underneath it. The files would look stored, the mount would hide them on
	// the next start, and none of them would ever reach the bucket — so this
	// is a refusal rather than a best effort.
	if mounted := b.command(ctx, "mountpoint", "-q", "--", b.mountpoint); mounted.Error != "" {
		return appplugin.Errorf(
			http.StatusConflict, "Nothing is mounted at %s", b.mountpoint)
	}

	destination := path.Join(b.mountpoint, b.uploadsDir)
	if made := b.command(ctx, "mkdir", "-p", "--", destination); made.Error != "" {
		return appplugin.Errorf(
			http.StatusBadGateway, "Could not create %s: %s", destination, failureText(made))
	}

	results := make([]pushResult, 0, len(request.Names))
	stored, removed := 0, 0
	for _, name := range request.Names {
		result := b.pushOne(ctx, destination, name)
		if result.Stored {
			stored++
		}
		if result.Removed {
			removed++
		}
		results = append(results, result)
	}
	return appplugin.JSON(http.StatusOK, map[string]any{
		"mountpoint": b.mountpoint,
		"directory":  destination,
		"stored":     stored,
		"removed":    removed,
		"results":    results,
	})
}

// pushOne moves one finished upload onto the mount: copy first, then delete
// the copy Remote left in .uploads.
//
// The order is the whole safety argument. The mount writes back synchronously
// unless --async-writeback is set, so cp returning means the object is in the
// bucket rather than merely in a local cache — and only then is deleting the
// other copy something other than throwing away the file.
func (b *backend) pushOne(ctx context.Context, destination, name string) pushResult {
	if err := validUploadName(name); err != nil {
		return pushResult{Name: name, Error: err.Error()}
	}
	source := path.Join(uploadsSource, name)
	result := pushResult{Name: name, Path: path.Join(destination, name)}

	// Checked rather than left to `cp -n`, which skips silently and exits 0:
	// that would make "already on the bucket" and "copied just now"
	// indistinguishable, and re-uploading an attachment on every reconnect is
	// exactly what this route should not do.
	if existing := b.command(ctx, "test", "-e", result.Path); existing.Error == "" {
		result.Skipped = true
	} else if copied := b.command(ctx, "cp", "--", source, result.Path); copied.Error != "" {
		result.Error = failureText(copied)
		return result
	}
	result.Stored = true

	if b.asyncWriteback {
		// The bytes may still be only in the local write-back cache, so the
		// copy in .uploads is not redundant yet and deleting it would be a
		// bet on that cache surviving. Flush the mount and push again.
		result.Error = "kept in .uploads: the mount is configured with --async-writeback, " +
			"so the object is not confirmed on S3 yet"
		return result
	}
	if deleted := b.command(ctx, "rm", "-f", "--", source); deleted.Error != "" {
		result.Error = failureText(deleted)
		return result
	}
	result.Removed = true
	return result
}

// validUploadName refuses anything that is not a plain file name. The plugin
// owns both directories, so a name carrying a path has no legitimate meaning
// here; cleaning it into one would silently accept a caller reaching for a
// file it was never offered.
func validUploadName(name string) error {
	switch {
	case name == "" || name == "." || name == "..":
		return fmt.Errorf("not a file name")
	case len(name) > 255:
		return fmt.Errorf("file name is too long")
	case strings.ContainsAny(name, "/\\\x00"):
		return fmt.Errorf("a file name may not contain a path separator")
	case strings.HasPrefix(name, "-"):
		// cp and mkdir take the name as an operand after --, but a leading
		// dash is still refused: it is never a name Remote stored, and
		// accepting it would rest on every later command keeping that guard.
		return fmt.Errorf("a file name may not start with a dash")
	}
	return nil
}

// failureText prefers what the command printed over the exit status, which on
// its own says nothing a reader can act on.
func failureText(result commandResult) string {
	text := result.Output
	if text == "" {
		text = result.Error
	}
	if len(text) > maxFailureText {
		text = text[:maxFailureText] + "…"
	}
	return text
}

// Discard excess output while continuing to drain the process pipes.
type limitedOutput struct{ data []byte }

func (w *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 64*1024 - len(w.data); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		w.data = append(w.data, p...)
	}
	return n, nil
}
func runContainer(ctx context.Context, container string, args ...string) commandResult {
	binary, err := exec.LookPath("lxc")
	if err != nil {
		binary = "/snap/bin/lxc"
	}
	commandArgs := append([]string{"exec", container, "--"}, args...)
	cmd := exec.CommandContext(ctx, binary, commandArgs...)
	cmd.WaitDelay = time.Second
	var out limitedOutput
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	result := commandResult{Output: strings.TrimSpace(string(out.data))}
	if ctx.Err() != nil {
		result.Error = "Command timed out; check mount status before retrying"
	} else if err != nil {
		result.Error = err.Error()
	}
	return result
}
