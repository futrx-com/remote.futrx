// Package api translates File Management backend requests into workspace
// operations. Remote has already authenticated the caller and authorized the
// chat before the trusted workspace root reaches this package.
package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	appWorkspace "futrx.local/catalog/applications/file-management/backend/workspace"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type api struct {
	router *applications.Router
	files  *appWorkspace.Service

	mu      sync.RWMutex
	spooler *appWorkspace.Spooler
}

var _ applications.Backend = (*api)(nil)

func New() applications.Backend {
	b := &api{
		router: applications.NewRouter(),
		files:  appWorkspace.NewService(appWorkspace.NewStore()),
	}
	b.router.GET("files", "List one workspace directory", b.list)
	b.router.GET("files/search", "Search workspace entry names", b.search)
	b.router.GET("files/download", "Download one workspace file", b.download)
	b.router.GET("files/download-folder", "Download one workspace folder as ZIP", b.downloadFolder)
	b.router.GET("files/media", "Open supported workspace media inline", b.media)
	return b
}

func (b *api) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{
		APIVersion: applications.APIVersion,
		Routes:     b.router.Routes(),
	}, nil
}

func (b *api) Init(instance applications.Instance) error {
	spooler, err := appWorkspace.NewSpooler(
		filepath.Join(instance.DataDir, "spool"),
		appWorkspace.MaxConcurrentArchives,
		appWorkspace.MaxArchiveBytes,
	)
	if err != nil {
		return fmt.Errorf("initialize archive spool: %w", err)
	}
	b.mu.Lock()
	b.spooler = spooler
	b.mu.Unlock()
	return nil
}

func (b *api) Handle(request applications.Request) (applications.Response, error) {
	return b.router.Serve(request), nil
}

func (b *api) list(request applications.Request) applications.Response {
	root, response, ok := chatWorkspace(request)
	if !ok {
		return response
	}
	listing, err := b.files.List(root, queryValue(request, "path"))
	if err != nil {
		return workspaceError(err)
	}
	return applications.JSON(http.StatusOK, listing)
}

func (b *api) search(request applications.Request) applications.Response {
	root, response, ok := chatWorkspace(request)
	if !ok {
		return response
	}
	result, err := b.files.Search(root, queryValue(request, "q"))
	if err != nil {
		return workspaceError(err)
	}
	return applications.JSON(http.StatusOK, result)
}

func (b *api) download(request applications.Request) applications.Response {
	root, response, ok := chatWorkspace(request)
	if !ok {
		return response
	}
	file, err := b.files.OpenFile(root, queryValue(request, "path"))
	if err != nil {
		return workspaceError(err)
	}
	return streamedContent(file, file.Size, file.Name, file.ModTime, "attachment", "")
}

func (b *api) media(request applications.Request) applications.Response {
	root, response, ok := chatWorkspace(request)
	if !ok {
		return response
	}
	file, err := b.files.OpenMedia(root, queryValue(request, "path"))
	if err != nil {
		return workspaceError(err)
	}
	response = streamedContent(file, file.Size, file.Name, file.ModTime, "inline", file.ContentType)
	if response.Status == http.StatusOK {
		response.Headers["Content-Security-Policy"] = []string{
			"default-src 'none'; img-src 'self' data: blob:; media-src 'self' data: blob:; style-src 'unsafe-inline'",
		}
	}
	return response
}

func (b *api) downloadFolder(request applications.Request) applications.Response {
	root, response, ok := chatWorkspace(request)
	if !ok {
		return response
	}
	archive, err := b.files.PrepareArchive(root, queryValue(request, "path"))
	if err != nil {
		return workspaceError(err)
	}
	b.mu.RLock()
	spooler := b.spooler
	b.mu.RUnlock()
	if spooler == nil {
		return applications.Errorf(http.StatusServiceUnavailable, "archive spool is not initialized")
	}

	// Request does not yet carry a cancellation context across the RPC boundary.
	// The process-level backend timeout remains the outer bound until streamed
	// responses add disconnect propagation.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	spooled, err := spooler.Prepare(ctx, func(destination io.Writer) error {
		return b.files.WriteArchive(ctx, archive, destination)
	})
	if err != nil {
		return workspaceError(err)
	}
	return streamedContent(
		spooled, spooled.Size(), archive.Name, time.Time{}, "attachment", "application/zip",
	)
}

func chatWorkspace(request applications.Request) (string, applications.Response, bool) {
	chat := request.Context.Chat
	if chat == nil || chat.ID == "" || chat.WorkspaceRoot == "" {
		return "", applications.Errorf(http.StatusBadRequest, "chat workspace context is required"), false
	}
	return chat.WorkspaceRoot, applications.Response{}, true
}

func queryValue(request applications.Request, name string) string {
	values := request.Query[name]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func streamedContent(
	content io.ReadSeekCloser,
	size int64,
	name string,
	modTime time.Time,
	disposition string,
	contentType string,
) applications.Response {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	headers := map[string][]string{
		"Content-Type":        {contentType},
		"Content-Disposition": {mime.FormatMediaType(disposition, map[string]string{"filename": name})},
	}
	return applications.Stream(content, size, modTime, headers)
}

func workspaceError(err error) applications.Response {
	switch {
	case errors.Is(err, appWorkspace.ErrArchiveTooLarge):
		return applications.Errorf(http.StatusRequestEntityTooLarge, "%v", err)
	case errors.Is(err, appWorkspace.ErrInvalidPath):
		return applications.Errorf(http.StatusBadRequest, "%v", err)
	case errors.Is(err, appWorkspace.ErrFileNotFound), errors.Is(err, appWorkspace.ErrFolderNotFound):
		return applications.Errorf(http.StatusNotFound, "%v", err)
	case errors.Is(err, appWorkspace.ErrUnsupportedMedia):
		return applications.Errorf(http.StatusUnsupportedMediaType, "%v", err)
	default:
		return applications.Errorf(http.StatusInternalServerError, "%v", err)
	}
}
