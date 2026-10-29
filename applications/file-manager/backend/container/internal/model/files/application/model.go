package application

import (
	"io"
	"time"
)

type Node struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime int64
}

// Listing is the contents of a single directory within the workspace.
type Listing struct {
	Path      string
	Entries   []*Node
	Truncated bool
}

// SearchResult is a flat set of workspace entries whose name matched a query.
type SearchResult struct {
	Entries   []*Node
	Truncated bool
}

type File struct {
	Name    string
	ModTime time.Time
	content io.ReadSeekCloser
}

func (f *File) Content() io.ReadSeeker {
	return f.content
}

func (f *File) Close() error {
	return f.content.Close()
}

type Archive struct {
	Name     string
	Root     string
	Relative string
}

func NewFile(name string, modTime time.Time, content io.ReadSeekCloser) *File {
	return &File{Name: name, ModTime: modTime, content: content}
}
