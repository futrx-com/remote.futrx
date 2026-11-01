package workspace

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

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

	archive := zip.NewWriter(destination)
	budget := archiveBudget{
		remainingBytes:   limits.maxSourceBytes,
		remainingEntries: limits.maxEntries,
	}

	// WalkDir does not descend into symlinked directories, so directory-symlink
	// loops are impossible. Symlinked files are included only if they still
	// resolve inside the workspace.
	walkErr := fs.WalkDir(workspace.root.FS(), base, func(walkPath string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if walkPath != base {
			if err := budget.visitEntry(); err != nil {
				return err
			}
		}
		if entry.IsDir() {
			return nil
		}
		return workspace.writeArchiveEntry(ctx, archive, &budget, base, walkPath, entry)
	})
	closeErr := archive.Close()
	return errors.Join(walkErr, closeErr, ctx.Err())
}

func (w secureWorkspace) writeArchiveEntry(
	ctx context.Context,
	archive *zip.Writer,
	budget *archiveBudget,
	base string,
	walkPath string,
	entry fs.DirEntry,
) error {
	openPath := walkPath
	if entry.Type()&os.ModeSymlink != 0 {
		target, err := w.resolve(walkPath)
		if err != nil {
			// Broken and escaping symlinks are intentionally omitted: neither is a
			// usable file inside the workspace boundary.
			return nil
		}
		openPath = target
	}

	source, info, err := w.openRegular(openPath)
	if err != nil {
		if errors.Is(err, errNotRegularFile) {
			return nil
		}
		return err
	}
	if err := budget.acceptFile(info.Size()); err != nil {
		return errors.Join(err, source.Close())
	}
	relative, err := filepath.Rel(base, walkPath)
	if err != nil {
		return errors.Join(err, source.Close())
	}
	destination, err := archive.Create(filepath.ToSlash(relative))
	if err != nil {
		return errors.Join(err, source.Close())
	}
	copyErr := budget.copy(ctx, destination, source)
	return errors.Join(copyErr, source.Close())
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
