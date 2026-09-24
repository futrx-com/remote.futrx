package hostfs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/shared/workspacepath"
)

var (
	errOutsideWorkspace = errors.New("path escapes workspace")
	errNotRegularFile   = errors.New("not a regular file")
)

// WorkspaceFileStore is retained for core's chat-message media endpoint. File
// browsing, search, and downloads live in the File Management application.
type WorkspaceFileStore struct{}

func NewWorkspaceFileStore() *WorkspaceFileStore {
	return &WorkspaceFileStore{}
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

// secureWorkspace owns the workspace containment boundary for one open. The
// os.Root handle keeps the final operation rooted even if a parent symlink is
// swapped after validation.
type secureWorkspace struct {
	root     *os.Root
	realRoot string
}

func newSecureWorkspace(root string) (secureWorkspace, error) {
	cleanRoot := filepath.Clean(root)
	realRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return secureWorkspace{}, err
	}
	rootHandle, err := os.OpenRoot(realRoot)
	if err != nil {
		return secureWorkspace{}, err
	}
	return secureWorkspace{root: rootHandle, realRoot: realRoot}, nil
}

func (w secureWorkspace) close() {
	_ = w.root.Close()
}

func (w secureWorkspace) openRegular(name string) (*os.File, os.FileInfo, error) {
	info, err := w.root.Stat(name)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errNotRegularFile
	}

	file, err := w.root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !openedInfo.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, errNotRegularFile
	}
	return file, openedInfo, nil
}

func (w secureWorkspace) resolve(relative string) (string, error) {
	relative = filepath.Join(string(filepath.Separator), filepath.FromSlash(relative))
	target := filepath.Join(w.realRoot, relative)
	if !workspacepath.Contains(target, w.realRoot) {
		return "", errOutsideWorkspace
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", err
	}
	if !workspacepath.Contains(resolved, w.realRoot) {
		return "", errOutsideWorkspace
	}
	resolvedRelative, err := filepath.Rel(w.realRoot, resolved)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(resolvedRelative), nil
}
