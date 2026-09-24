package workspacefiles

import (
	"io"
	"time"
)

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

type Media struct {
	File        *File
	ContentType string
}
