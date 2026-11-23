package api

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
)

// Remote writes project chat attachments beside the container's mount.
const uploadsSource = "/workspace/.uploads"

const maxFailureText = 500

var errNotMounted = errors.New("mountpoint is not mounted")

// attachmentRun executes one command inside the installed project's container.
// The caller supplies an adapter that redacts credentials before returning output.
type attachmentRun func(context.Context, ...string) (output, errorText string)

// attachmentCopier owns the order of the mount check, copy, and source deletion.
type attachmentCopier struct {
	mountpoint     string
	uploadsDir     string
	asyncWriteback bool
	run            attachmentRun
}

func newAttachmentCopier(mountpoint, uploadsDir string, asyncWriteback bool, run attachmentRun) *attachmentCopier {
	return &attachmentCopier{mountpoint: mountpoint, uploadsDir: uploadsDir, asyncWriteback: asyncWriteback, run: run}
}

type attachmentResult struct {
	Name    string
	Path    string
	Stored  bool
	Removed bool
	Skipped bool
	Error   string
}

type attachmentBatch struct {
	Directory string
	Stored    int
	Removed   int
	Results   []attachmentResult
}

func (c *attachmentCopier) push(ctx context.Context, names []string) (attachmentBatch, error) {
	// A copy into the plain directory beneath an unmounted mount would appear
	// stored while never reaching the bucket.
	if _, commandError := c.run(ctx, "mountpoint", "-q", "--", c.mountpoint); commandError != "" {
		return attachmentBatch{}, errNotMounted
	}

	destination := path.Join(c.mountpoint, c.uploadsDir)
	if output, commandError := c.run(ctx, "mkdir", "-p", "--", destination); commandError != "" {
		return attachmentBatch{}, fmt.Errorf("Could not create %s: %s", destination, failureText(output, commandError))
	}

	batch := attachmentBatch{Directory: destination, Results: make([]attachmentResult, 0, len(names))}
	for _, name := range names {
		result := c.pushOne(ctx, destination, name)
		if result.Stored {
			batch.Stored++
		}
		if result.Removed {
			batch.Removed++
		}
		batch.Results = append(batch.Results, result)
	}
	return batch, nil
}

// pushOne copies first, then deletes the upload only after the mount confirms
// the object is stored. A synchronous writeback makes that order safe.
func (c *attachmentCopier) pushOne(ctx context.Context, destination, name string) attachmentResult {
	if err := validUploadName(name); err != nil {
		return attachmentResult{Name: name, Error: err.Error()}
	}
	source := path.Join(uploadsSource, name)
	result := attachmentResult{Name: name, Path: path.Join(destination, name)}

	// cp -n would silently skip an existing object; probe it so the caller can
	// distinguish an existing object from a new copy.
	if _, commandError := c.run(ctx, "test", "-e", result.Path); commandError == "" {
		result.Skipped = true
	} else if output, commandError := c.run(ctx, "cp", "--", source, result.Path); commandError != "" {
		result.Error = failureText(output, commandError)
		return result
	}
	result.Stored = true

	if c.asyncWriteback {
		result.Error = "kept in .uploads: the mount is configured with --async-writeback, " +
			"so the object is not confirmed on S3 yet"
		return result
	}
	if output, commandError := c.run(ctx, "rm", "-f", "--", source); commandError != "" {
		result.Error = failureText(output, commandError)
		return result
	}
	result.Removed = true
	return result
}

func validUploadName(name string) error {
	switch {
	case name == "" || name == "." || name == "..":
		return fmt.Errorf("not a file name")
	case len(name) > 255:
		return fmt.Errorf("file name is too long")
	case strings.ContainsAny(name, "/\\\x00"):
		return fmt.Errorf("a file name may not contain a path separator")
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("a file name may not start with a dash")
	}
	return nil
}

func failureText(output, commandError string) string {
	text := output
	if text == "" {
		text = commandError
	}
	if len(text) > maxFailureText {
		text = text[:maxFailureText] + "…"
	}
	return text
}
