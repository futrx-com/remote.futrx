package outbound

import (
	"context"
	"io"

	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
)

// Store provides rooted file operations. Implementations enforce containment,
// including symlinks and concurrent path changes.
type Store interface {
	DirectoryExists(root, relative string) bool
	ListDir(root, relative string, maxEntries int) ([]*model.Node, bool, error)
	WriteArchive(ctx context.Context, root, relative string, destination io.Writer) error
	Search(root, query string, limit int) ([]*model.Node, bool, error)
}

// ArchiveDownloads stores a finished archive where the browser can fetch it
// and returns its workspace-relative path. A failed build stores nothing.
type ArchiveDownloads interface {
	Publish(ctx context.Context, name string, write func(io.Writer) error) (string, error)
}
