// Package workspace owns File Management's filesystem policy and its
// symlink-safe access to one trusted chat workspace.
package workspace

import (
	"errors"
	"io"
	"time"
)

var (
	ErrInvalidPath      = errors.New("invalid path")
	ErrFileNotFound     = errors.New("file not found")
	ErrFolderNotFound   = errors.New("folder not found")
	ErrUnsupportedMedia = errors.New("file type cannot be opened in browser")
	ErrArchiveTooLarge  = errors.New("workspace archive exceeds download size limit")
)

type Node struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size,omitempty"`
	ModTime int64  `json:"modTime,omitempty"`
}

type Listing struct {
	Path      string  `json:"path"`
	Entries   []*Node `json:"entries"`
	Truncated bool    `json:"truncated"`
}

type SearchResult struct {
	Entries   []*Node `json:"entries"`
	Truncated bool    `json:"truncated"`
}

type File struct {
	Name        string
	Size        int64
	ModTime     time.Time
	ContentType string
	content     io.ReadSeekCloser
}

func (f *File) Content() io.ReadSeeker          { return f }
func (f *File) Read(buffer []byte) (int, error) { return f.content.Read(buffer) }
func (f *File) Seek(offset int64, whence int) (int64, error) {
	return f.content.Seek(offset, whence)
}
func (f *File) Close() error { return f.content.Close() }

type Archive struct {
	Name     string
	root     string
	relative string
}
