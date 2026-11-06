// Package cli is the container side of File Manager's host backend. The host
// runs one command per request through `lxc exec`, writes the request as JSON
// on stdin, and reads the result from stdout. Failures are reported through
// the exit status so the host can map them without parsing output.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/port/files/inbound"
)

// Exit statuses shared with backend/containerio. The host and container are
// separate modules, so both sides spell them out.
const (
	ExitOK          = 0
	ExitInternal    = 1
	ExitUsage       = 2
	ExitInvalidPath = 3
	ExitNotFound    = 4
	ExitTooLarge    = 5
)

// request is what the host sends on stdin. Browser input arrives here rather
// than as program arguments.
type request struct {
	Path  string `json:"path"`
	Query string `json:"query"`
}

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

// Run executes the command named by args[0] and returns the process status.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, files inbound.Files) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: file-manager list|search|archive < request.json")
		return ExitUsage
	}
	var input request
	decoder := json.NewDecoder(io.LimitReader(stdin, config.MaxRequestBytes))
	if err := decoder.Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintln(stderr, "invalid request:", err)
		return ExitUsage
	}

	var err error
	switch args[0] {
	case "list":
		err = list(stdout, files, input.Path)
	case "search":
		err = search(stdout, files, input.Query)
	case "archive":
		err = archive(ctx, stdout, files, input.Path)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		return ExitUsage
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitStatus(err)
	}
	return ExitOK
}

func list(stdout io.Writer, files inbound.Files, path string) error {
	result, err := files.List(path)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(listing{Path: result.Path, Entries: presentNodes(result.Entries), Truncated: result.Truncated})
}

func search(stdout io.Writer, files inbound.Files, query string) error {
	result, err := files.Search(query)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(searchResult{Entries: presentNodes(result.Entries), Truncated: result.Truncated})
}

// archive writes nothing until the bounded archive is complete, so a failed
// build never looks like the start of a ZIP.
func archive(ctx context.Context, stdout io.Writer, files inbound.Files, path string) error {
	file, err := files.DownloadArchive(ctx, path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(stdout, file.Content())
	return err
}

func presentNodes(entries []*model.Node) []*node {
	nodes := make([]*node, 0, len(entries))
	for _, entry := range entries {
		nodes = append(nodes, &node{Name: entry.Name, Path: entry.Path, IsDir: entry.IsDir, Size: entry.Size, ModTime: entry.ModTime})
	}
	return nodes
}

func exitStatus(err error) int {
	switch {
	case errors.Is(err, model.ErrInvalidPath):
		return ExitInvalidPath
	case errors.Is(err, model.ErrFileNotFound), errors.Is(err, model.ErrFolderNotFound):
		return ExitNotFound
	case errors.Is(err, model.ErrArchiveTooLarge):
		return ExitTooLarge
	}
	return ExitInternal
}
