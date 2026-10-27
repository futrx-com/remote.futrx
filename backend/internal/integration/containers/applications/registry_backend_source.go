package applications

import (
	"io"
	"io/fs"
	"strings"
)

// hostBackendSource exposes backend/ to the host compiler while making the
// container-only subtree absent. Keeping that boundary in the filesystem
// handed to the builder means container programs cannot enter a host build,
// its fingerprint, or its generated module accidentally.
type hostBackendSource struct {
	fs.FS
}

func (source hostBackendSource) Open(name string) (fs.File, error) {
	if isContainerSourcePath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	file, err := source.FS.Open(name)
	if err != nil || name != "." {
		return file, err
	}
	directory, ok := file.(fs.ReadDirFile)
	if !ok {
		return file, nil
	}
	return &hostBackendRoot{File: file, directory: directory}, nil
}

func (source hostBackendSource) ReadDir(name string) ([]fs.DirEntry, error) {
	if isContainerSourcePath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	entries, err := fs.ReadDir(source.FS, name)
	if err != nil || name != "." {
		return entries, err
	}
	return withoutContainerEntry(entries), nil
}

func isContainerSourcePath(name string) bool {
	return name == backendContainerDir || strings.HasPrefix(name, backendContainerDir+"/")
}

func withoutContainerEntry(entries []fs.DirEntry) []fs.DirEntry {
	filtered := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() != backendContainerDir {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// hostBackendRoot preserves fs.ReadDirFile's batched semantics while skipping
// the one top-level directory owned by the container build.
type hostBackendRoot struct {
	fs.File
	directory fs.ReadDirFile
}

func (root *hostBackendRoot) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		entries, err := root.directory.ReadDir(n)
		return withoutContainerEntry(entries), err
	}

	entries := make([]fs.DirEntry, 0, n)
	for len(entries) < n {
		batch, err := root.directory.ReadDir(n - len(entries))
		entries = append(entries, withoutContainerEntry(batch)...)
		if err != nil {
			if err == io.EOF && len(entries) > 0 {
				return entries, nil
			}
			return entries, err
		}
		if len(batch) == 0 {
			if len(entries) > 0 {
				return entries, nil
			}
			return nil, io.EOF
		}
	}
	return entries, nil
}
