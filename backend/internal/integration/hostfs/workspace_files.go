package hostfs

import (
	"errors"
	"github.com/futrx-com/remote.futrx.com/internal/shared/workspacepath"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

var (
	errOutsideWorkspace = errors.New("path escapes workspace")
	errNotRegularFile   = errors.New("not a regular file")
)

type WorkspaceFileStore struct{}

// secureWorkspace owns the workspace containment boundary for one store
// operation. The os.Root handle makes the final filesystem operation relative
// to an opened directory, so a concurrent symlink swap cannot redirect it
// outside the workspace after path validation.
type secureWorkspace struct {
	root     *os.Root
	realRoot string
}

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

// openRegular checks before opening to avoid touching known special files, then
// opens nonblocking and verifies the resulting descriptor. The second check
// closes the race where a regular path is swapped for a FIFO or device between
// Stat and OpenFile.
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

// resolve joins relative under root, collapses traversal, then resolves
// symlinks and translates the result to an os.Root-relative name. The final
// open/stat still goes through os.Root, which enforces containment if the path
// changes after this resolution. Resolving here preserves support for absolute
// symlinks whose targets remain inside the workspace; os.Root intentionally
// rejects absolute symlinks when asked to follow them directly.
func (w secureWorkspace) resolve(relative string) (string, error) {
	rel := filepath.Join(string(filepath.Separator), filepath.FromSlash(relative))
	target := filepath.Join(w.realRoot, rel)
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
