package workspace

import (
	"errors"
	"io"
	"os"
	"time"
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

func (s *WorkspaceFileStore) OpenFile(root, relative string) (io.ReadSeekCloser, string, time.Time, error) {
	workspace, err := newSecureWorkspace(root)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	defer workspace.close()
	resolved, err := workspace.resolve(relative)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	file, info, err := workspace.openRegular(resolved)
	if err != nil {
		if errors.Is(err, errNotRegularFile) {
			return nil, "", time.Time{}, os.ErrNotExist
		}
		return nil, "", time.Time{}, err
	}
	return file, info.Name(), info.ModTime(), nil
}
