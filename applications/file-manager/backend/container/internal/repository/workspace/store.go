package workspace

import (
	"io/fs"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
)

type WorkspaceFileStore struct{}

func New() *WorkspaceFileStore { return &WorkspaceFileStore{} }

func (s *WorkspaceFileStore) DirectoryExists(root, relative string) bool {
	workspace, err := newSecureWorkspace(root)
	if err != nil {
		return false
	}
	defer workspace.close()
	resolved, err := workspace.resolve(relative)
	if err != nil {
		return false
	}
	info, err := workspace.root.Stat(resolved)
	return err == nil && info.IsDir()
}

// isDownloads reports the folder finished ZIPs wait in. It is File Manager's
// own scratch space, so the browser never lists, searches or re-archives it.
func isDownloads(rootRelative string, entry fs.DirEntry) bool {
	return rootRelative == config.DownloadsDirectory && entry.IsDir()
}
