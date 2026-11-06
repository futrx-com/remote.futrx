// Package api serves File Manager's workspace routes. Each route runs the
// container program once; Remote's project routes have already checked that
// the caller is a member of this installation's project.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"

	"futrx.local/catalog/applications/file-manager/backend/config"
	"futrx.local/catalog/applications/file-manager/backend/containerio"
	"futrx.local/catalog/applications/file-manager/backend/service/archives"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
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

type API struct {
	router   *applications.Router
	run      containerio.Runner
	archives *archives.Downloads

	mu        sync.Mutex
	container string
}

var _ applications.Backend = (*API)(nil)

func New(run containerio.Runner, downloads *archives.Downloads) *API {
	b := &API{
		router:   applications.NewRouter(),
		run:      run,
		archives: downloads,
	}
	b.router.GET("list", "List one workspace folder", b.list)
	b.router.GET("search", "Search workspace file names", b.search)
	b.router.GET("archive", "Download a workspace folder as a ZIP", b.archive)
	return b
}

func (b *API) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{APIVersion: applications.APIVersion, Routes: b.router.Routes()}, nil
}

func (b *API) Init(instance applications.Instance) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.container = instance.ContainerName
	return nil
}

func (b *API) Handle(request applications.Request) (applications.Response, error) {
	return b.router.Serve(request), nil
}

func (b *API) list(request applications.Request) applications.Response {
	var result listing
	if response, ok := b.query("list", containerio.Request{Path: first(request, "path")}, &result); !ok {
		return response
	}
	if result.Entries == nil {
		result.Entries = []*node{}
	}
	return applications.JSON(http.StatusOK, result)
}

func (b *API) search(request applications.Request) applications.Response {
	var result searchResult
	if response, ok := b.query("search", containerio.Request{Query: first(request, "q")}, &result); !ok {
		return response
	}
	if result.Entries == nil {
		result.Entries = []*node{}
	}
	return applications.JSON(http.StatusOK, result)
}

// query runs a JSON command and decodes it into result. The container is the
// project's, so its output is re-encoded through typed values rather than
// passed through.
func (b *API) query(command string, input containerio.Request, result any) (applications.Response, bool) {
	container, response, ok := b.target(input)
	if !ok {
		return response, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.ListTimeout)
	defer cancel()
	output, err := b.run(ctx, container, command, input, config.MaxListingBytes)
	if err != nil {
		return failure(err), false
	}
	if err := json.Unmarshal(output, result); err != nil {
		return applications.Errorf(http.StatusBadGateway, "Unreadable file listing from the project container."), false
	}
	return applications.Response{}, true
}

func (b *API) archive(request applications.Request) applications.Response {
	relative := cleanRelative(first(request, "path"))
	container, response, ok := b.target(containerio.Request{Path: relative})
	if !ok {
		return response
	}
	content, err := b.archives.Build(func(ctx context.Context) ([]byte, error) {
		return b.run(ctx, container, "archive", containerio.Request{Path: relative}, config.MaxArchiveBytes)
	})
	if err != nil {
		return failure(err)
	}
	return applications.Response{
		Status: http.StatusOK,
		Headers: map[string][]string{
			"Content-Type":        {"application/zip"},
			"Content-Disposition": {mime.FormatMediaType("attachment", map[string]string{"filename": archiveName(relative)})},
		},
		Body: content,
	}
}

func (b *API) target(input containerio.Request) (string, applications.Response, bool) {
	if len(input.Path) > config.MaxQueryBytes || len(input.Query) > config.MaxQueryBytes {
		return "", applications.Errorf(http.StatusBadRequest, "The path or search is too long."), false
	}
	b.mu.Lock()
	container := b.container
	b.mu.Unlock()
	if container == "" {
		return "", applications.Errorf(http.StatusConflict, "This installation has no project container."), false
	}
	return container, applications.Response{}, true
}

func failure(err error) applications.Response {
	switch {
	case errors.Is(err, archives.ErrBusy):
		return applications.Errorf(http.StatusServiceUnavailable, "Other downloads are still being prepared. Try again shortly.")
	case errors.Is(err, containerio.ErrInvalidPath):
		return applications.Errorf(http.StatusBadRequest, "Invalid path.")
	case errors.Is(err, containerio.ErrNotFound):
		return applications.Errorf(http.StatusNotFound, "Not found.")
	case errors.Is(err, containerio.ErrTooLarge):
		return applications.Errorf(http.StatusRequestEntityTooLarge, "This folder is too large to download as a ZIP.")
	case errors.Is(err, context.DeadlineExceeded):
		return applications.Errorf(http.StatusGatewayTimeout, "The project container took too long to answer.")
	}
	return applications.Errorf(http.StatusBadGateway, "Could not reach File Manager in the project container: %v", err)
}

func first(request applications.Request, key string) string {
	if values := request.Query[key]; len(values) > 0 {
		return values[0]
	}
	return ""
}

// cleanRelative mirrors the container's normalisation so the download is
// named after the folder that is actually archived. Containment is enforced
// inside the container.
func cleanRelative(relativePath string) string {
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" {
		return ""
	}
	return strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(relativePath, "/")), "/")
}

func archiveName(relative string) string {
	name := path.Base(relative)
	if relative == "" || name == "." || name == "/" {
		name = "workspace"
	}
	return name + ".zip"
}
