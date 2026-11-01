package httphandlers

import (
	"errors"
	"net/http"

	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
)

type node struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size,omitempty"`
	ModTime int64  `json:"modTime,omitempty"`
}

type listing struct {
	Path      string  `json:"path"`
	Entries   []*node `json:"entries"`
	Truncated bool    `json:"truncated"`
}

type searchResult struct {
	Entries   []*node `json:"entries"`
	Truncated bool    `json:"truncated"`
}

func presentNodes(entries []*model.Node) []*node {
	nodes := make([]*node, 0, len(entries))
	for _, entry := range entries {
		nodes = append(nodes, &node{Name: entry.Name, Path: entry.Path, IsDir: entry.IsDir, Size: entry.Size, ModTime: entry.ModTime})
	}
	return nodes
}

func sendError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, model.ErrInvalidPath):
		code = http.StatusBadRequest
	case errors.Is(err, model.ErrFileNotFound), errors.Is(err, model.ErrFolderNotFound):
		code = http.StatusNotFound
	case errors.Is(err, model.ErrArchiveTooLarge):
		code = http.StatusRequestEntityTooLarge
	}
	http.Error(w, http.StatusText(code), code)
}
