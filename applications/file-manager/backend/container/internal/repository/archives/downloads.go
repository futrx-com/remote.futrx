// Package archives keeps finished folder ZIPs in the workspace, where Remote's
// chat download route streams them to the browser. Nothing here serves bytes.
package archives

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"syscall"
	"time"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
)

var errNotDirectory = errors.New(config.DownloadsDirectory + " is not a directory")

type Downloads struct {
	root      string
	maxBytes  int64
	retention time.Duration
	now       func() time.Time
}

func New(root string, maxBytes int64, retention time.Duration) *Downloads {
	return &Downloads{root: root, maxBytes: maxBytes, retention: retention, now: time.Now}
}

// Publish writes one archive into its own folder under the downloads directory
// and returns its workspace-relative path. The ZIP takes its final name only
// once it is complete, and a failed build leaves nothing behind. Every
// operation goes through os.Root, so a symlinked downloads directory cannot
// redirect writes or deletions outside the workspace.
func (d *Downloads) Publish(ctx context.Context, name string, writeArchive func(io.Writer) error) (string, error) {
	workspace, err := os.OpenRoot(d.root)
	if err != nil {
		return "", err
	}
	defer workspace.Close()
	if err := prepareDirectory(workspace); err != nil {
		return "", err
	}
	d.prune(workspace)

	folder := path.Join(config.DownloadsDirectory, randomName())
	if err := workspace.Mkdir(folder, 0o755); err != nil {
		return "", err
	}
	published := false
	defer func() {
		if !published {
			_ = removeFolder(workspace, folder)
		}
	}()

	partial := path.Join(folder, ".partial")
	file, err := workspace.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	bounded := &boundedContextWriter{ctx: ctx, destination: file, remaining: d.maxBytes}
	writeErr := writeArchive(bounded)
	if err := errors.Join(writeErr, bounded.writeErr, file.Close(), ctx.Err()); err != nil {
		return "", err
	}
	if err := renameInFolder(workspace, folder, ".partial", name); err != nil {
		return "", err
	}
	published = true
	return path.Join(folder, name), nil
}

// removeFolder deletes one archive folder. It holds files only, so it is
// emptied through os.Root and then removed, never following a symlink out.
// (os.Root gained RemoveAll in Go 1.25; container programs build with 1.24.)
func removeFolder(workspace *os.Root, folder string) error {
	entries, err := fs.ReadDir(workspace.FS(), folder)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var errs []error
	for _, entry := range entries {
		errs = append(errs, workspace.Remove(path.Join(folder, entry.Name())))
	}
	if err := workspace.Remove(folder); err != nil && !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// renameInFolder renames one entry inside folder relative to the folder's own
// descriptor, which os.Root resolved inside the workspace, so no path outside
// it can be named. (os.Root gained Rename in Go 1.25.)
func renameInFolder(workspace *os.Root, folder, from, to string) error {
	directory, err := workspace.Open(folder)
	if err != nil {
		return err
	}
	defer directory.Close()
	descriptor := int(directory.Fd())
	if err := syscall.Renameat(descriptor, from, descriptor, to); err != nil {
		return &os.LinkError{Op: "renameat", Old: from, New: to, Err: err}
	}
	return nil
}

// prepareDirectory creates the downloads directory with a .gitignore that
// ignores everything in it, so archives never show up as project changes.
func prepareDirectory(workspace *os.Root) error {
	if err := workspace.Mkdir(config.DownloadsDirectory, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	info, err := workspace.Lstat(config.DownloadsDirectory)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errNotDirectory
	}
	ignore, err := workspace.OpenFile(path.Join(config.DownloadsDirectory, ".gitignore"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = io.WriteString(ignore, "*\n")
	return errors.Join(err, ignore.Close())
}

// prune deletes archive folders older than the retention. A download already
// streaming holds its file open, so deletion never cuts one short.
func (d *Downloads) prune(workspace *os.Root) {
	entries, err := fs.ReadDir(workspace.FS(), config.DownloadsDirectory)
	if err != nil {
		return
	}
	cutoff := d.now().Add(-d.retention)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = removeFolder(workspace, path.Join(config.DownloadsDirectory, entry.Name()))
	}
}

// randomName keeps one download's path from being guessed from another's.
func randomName() string {
	var id [16]byte
	_, _ = rand.Read(id[:])
	return hex.EncodeToString(id[:])
}

type boundedContextWriter struct {
	ctx         context.Context
	destination io.Writer
	remaining   int64
	writeErr    error
}

func (w *boundedContextWriter) Write(buffer []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if err := w.ctx.Err(); err != nil {
		w.writeErr = err
		return 0, err
	}

	allowed := buffer
	exceedsLimit := int64(len(buffer)) > w.remaining
	if exceedsLimit {
		allowed = buffer[:int(w.remaining)]
	}
	n, err := w.destination.Write(allowed)
	w.remaining -= int64(n)
	if err != nil {
		w.writeErr = err
		return n, err
	}
	if n != len(allowed) {
		w.writeErr = io.ErrShortWrite
		return n, io.ErrShortWrite
	}
	if exceedsLimit {
		w.writeErr = model.ErrArchiveTooLarge
		return n, model.ErrArchiveTooLarge
	}
	return n, nil
}
