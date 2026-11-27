package workspace

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
)

type archiveLimits struct {
	maxSourceBytes int64
	maxEntries     int
}

func (s *WorkspaceFileStore) WriteArchive(ctx context.Context, root, relative string, destination io.Writer) error {
	return s.writeArchive(ctx, root, relative, destination, archiveLimits{maxSourceBytes: config.MaxArchiveSourceBytes, maxEntries: config.MaxArchiveEntries})
}

func (s *WorkspaceFileStore) writeArchive(
	ctx context.Context,
	root string,
	relative string,
	destination io.Writer,
	limits archiveLimits,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	workspace, err := newSecureWorkspace(root)
	if err != nil {
		return err
	}
	defer workspace.close()
	base, err := workspace.resolve(relative)
	if err != nil {
		return err
	}

	walk := &archiveWalk{
		ctx:       ctx,
		workspace: workspace,
		archive:   zip.NewWriter(destination),
		budget: archiveBudget{
			remainingBytes:   limits.maxSourceBytes,
			remainingEntries: limits.maxEntries,
		},
		base:     path.Clean(base),
		expanded: map[string]bool{},
	}
	walkErr := walk.directory(base, "")
	closeErr := walk.archive.Close()
	return errors.Join(walkErr, closeErr, ctx.Err())
}

// archiveWalk writes one folder into a ZIP. Entries are named by the path
// under the chosen folder, including paths that pass through a symlink.
type archiveWalk struct {
	ctx       context.Context
	workspace secureWorkspace
	archive   *zip.Writer
	budget    archiveBudget
	// base is the chosen folder, by resolved path.
	base string
	// expanded holds every directory already written, by resolved path, so a
	// directory reached through several symlinks, or a symlink loop, is
	// written once.
	expanded map[string]bool
}

// directory writes dir's contents under prefix. fs.WalkDir does not descend
// into symlinked directories itself; entry follows each one that resolves
// inside the workspace, so content reached through a link, such as
// .claude/skills -> ../.agents/skills, is not silently left out.
func (a *archiveWalk) directory(dir, prefix string) error {
	return fs.WalkDir(a.workspace.root.FS(), dir, func(walkPath string, entry fs.DirEntry, walkErr error) error {
		if err := a.ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if isDownloads(walkPath, entry) {
			return fs.SkipDir
		}
		if walkPath == dir {
			return nil
		}
		if err := a.budget.visitEntry(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(dir, walkPath)
		if err != nil {
			return err
		}
		return a.entry(walkPath, path.Join(prefix, filepath.ToSlash(relative)), entry)
	})
}

func (a *archiveWalk) entry(walkPath, name string, entry fs.DirEntry) error {
	if entry.Type()&os.ModeSymlink == 0 {
		return a.file(walkPath, name)
	}
	target, err := a.workspace.resolve(walkPath)
	if err != nil {
		// Broken and escaping symlinks are intentionally omitted: neither is a
		// usable file inside the workspace boundary.
		return nil
	}
	info, err := a.workspace.root.Stat(target)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		return a.file(target, name)
	}
	target = path.Clean(target)
	// A link to the chosen folder or one above it would archive the folder
	// inside itself, or the whole workspace; it is left out like a loop.
	if a.expanded[target] || insideDownloads(target) || within(a.base, target) {
		return nil
	}
	a.expanded[target] = true
	return a.directory(target, name)
}

// file writes one regular file. Special files are skipped.
func (a *archiveWalk) file(openPath, name string) error {
	source, info, err := a.workspace.openRegular(openPath)
	if err != nil {
		if errors.Is(err, errNotRegularFile) {
			return nil
		}
		return err
	}
	if err := a.budget.acceptFile(info.Size()); err != nil {
		return errors.Join(err, source.Close())
	}
	destination, err := a.archive.Create(name)
	if err != nil {
		return errors.Join(err, source.Close())
	}
	copyErr := a.budget.copy(a.ctx, destination, source)
	return errors.Join(copyErr, source.Close())
}

// within reports whether rootRelative is directory or lies below it.
func within(rootRelative, directory string) bool {
	return directory == "." || rootRelative == directory || strings.HasPrefix(rootRelative, directory+"/")
}

func insideDownloads(rootRelative string) bool {
	return rootRelative == config.DownloadsDirectory ||
		strings.HasPrefix(rootRelative, config.DownloadsDirectory+"/")
}

type archiveBudget struct {
	remainingBytes   int64
	remainingEntries int
}

func (b *archiveBudget) visitEntry() error {
	if b.remainingEntries <= 0 {
		return model.ErrArchiveTooLarge
	}
	b.remainingEntries--
	return nil
}

func (b *archiveBudget) acceptFile(size int64) error {
	if size > b.remainingBytes {
		return model.ErrArchiveTooLarge
	}
	return nil
}

func (b *archiveBudget) copy(ctx context.Context, destination io.Writer, source io.Reader) error {
	reader := contextReader{ctx: ctx, reader: source}
	written, copyErr := io.Copy(destination, io.LimitReader(reader, b.remainingBytes+1))
	if written > b.remainingBytes {
		b.remainingBytes = 0
		return errors.Join(copyErr, model.ErrArchiveTooLarge)
	}
	b.remainingBytes -= written
	return copyErr
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
