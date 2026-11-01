package httptransport_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/presentation/web"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/repository/archives"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/repository/workspace"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/service/files"
	httptransport "futrx.local/catalog/applications/file-manager/backend/container/internal/transport/http"
	httphandlers "futrx.local/catalog/applications/file-manager/backend/container/internal/transport/http/handlers"
	httpmiddleware "futrx.local/catalog/applications/file-manager/backend/container/internal/transport/http/middleware"
)

func TestFileManagerRoutes(t *testing.T) {
	root, _ := setupWorkspace(t)
	handler := newHandler(root)
	for _, test := range []struct {
		path    string
		status  int
		content string
	}{
		{"/health", 200, "file-manager ready"},
		{"/", 200, "Search all files"},
		{"/api/download?path=src/app.go", 200, "package main"},
		{"/api/list?path=missing", 404, "Not Found"},
		{"/api/download?path=../outside-secret.txt", 404, "Not Found"},
		{"/api/archive?path=missing", 404, "Not Found"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", test.path, nil))
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.content) {
				t.Fatalf("response %d %s", response.Code, response.Body.String())
			}
			if response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing content type protection")
			}
		})
	}
	t.Run("listing", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/list", nil))
		var listing model.Listing
		if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
			t.Fatal(err)
		}
		if len(listing.Entries) != 2 || listing.Entries[0].Name != "src" || listing.Entries[1].Name != ".env" {
			t.Fatalf("listing: %+v", listing)
		}
	})
	t.Run("search", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/search?q=app", nil))
		var results model.SearchResult
		if err := json.Unmarshal(response.Body.Bytes(), &results); err != nil {
			t.Fatal(err)
		}
		if len(results.Entries) != 1 || results.Entries[0].Path != "src/app.go" {
			t.Fatalf("search: %+v", results)
		}
	})
	t.Run("archive", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/archive?path=src", nil))
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/zip" {
			t.Fatalf("archive: %d", response.Code)
		}
		archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
		if err != nil {
			t.Fatal(err)
		}
		if len(archive.File) != 1 || archive.File[0].Name != "app.go" {
			t.Fatalf("archive entries: %+v", archive.File)
		}
	})
}

func TestArchiveFailureIsReportedBeforeDownloadHeaders(t *testing.T) {
	root, _ := setupWorkspace(t)
	response := httptest.NewRecorder()
	newHandlerWithSpool(root, archives.New(1, 4)).ServeHTTP(response, httptest.NewRequest("GET", "/api/archive?path=src", nil))
	if response.Code != http.StatusRequestEntityTooLarge || response.Header().Get("Content-Type") == "application/zip" {
		t.Fatalf("oversized response: %d %v", response.Code, response.Header())
	}
}

func setupWorkspace(t *testing.T) (root, secret string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "workspace")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0o644); err != nil {
		t.Fatal(err)
	}
	secret = filepath.Join(base, "outside-secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, secret
}

func newHandler(root string) http.Handler {
	spool := archives.New(config.MaxConcurrentArchives, config.MaxArchiveSpoolBytes)
	return newHandlerWithSpool(root, spool)
}

func newHandlerWithSpool(root string, spool *archives.Spool) http.Handler {
	service := files.New(root, workspace.New(), spool)
	return httptransport.NewHandler(httptransport.Handlers{
		Files:      httphandlers.NewFileHandler(service),
		Static:     http.FileServer(http.FS(web.Files())),
		Middleware: httpmiddleware.PageHeaders{},
	})
}
