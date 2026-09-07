package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// Moving a finished chat attachment onto the mount. Remote writes attachments
// from the host, so they land beside the mount and never inside it; this runs
// in the container, where the FUSE mount exists, and copies before it deletes.

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
