package outbound

import (
	"context"
	"io"
	"time"

	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
)

// Store provides rooted file operations. Implementations enforce containment,
// including symlinks and concurrent path changes.
type Store interface {
	DirectoryExists(root, relative string) bool
	ListDir(root, relative string, maxEntries int) ([]*model.Node, bool, error)
	OpenFile(root, relative string) (io.ReadSeekCloser, string, time.Time, error)
	WriteArchive(ctx context.Context, root, relative string, destination io.Writer) error
	Search(root, query string, limit int) ([]*model.Node, bool, error)
}

// ArchiveSpool bounds temporary storage and concurrent archive downloads.
// Closing the returned content releases both the file and its admission slot.
type ArchiveSpool interface {
	Prepare(context.Context, func(io.Writer) error) (io.ReadSeekCloser, error)
}
