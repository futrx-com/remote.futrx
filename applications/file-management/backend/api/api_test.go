package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appWorkspace "futrx.local/catalog/applications/file-management/backend/workspace"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

func testBackend(t *testing.T) (*api, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"src/app.go": "package main",
		".env":       "SECRET=1",
		"pixel.png":  "png-data",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	backend := New().(*api)
	if err := backend.Init(applications.Instance{DataDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	return backend, root
}

func request(root, method, path string, query map[string][]string) applications.Request {
	return applications.Request{
		Method: method,
		Path:   path,
		Query:  query,
		Context: applications.RequestContext{Chat: &applications.ChatContext{
			ID: "chat-1", WorkspaceRoot: root,
		}},
	}
}

func TestRoutesRequireTrustedChatContext(t *testing.T) {
	backend, _ := testBackend(t)
	response, err := backend.Handle(applications.Request{Method: http.MethodGet, Path: "files"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusBadRequest || !strings.Contains(string(response.Body), "chat workspace context") {
		t.Fatalf("response = %d %s", response.Status, response.Body)
	}
}

func TestListAndSearchReturnExistingJSONShape(t *testing.T) {
	backend, root := testBackend(t)
	response, err := backend.Handle(request(root, http.MethodGet, "files", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusOK || response.Headers["Content-Type"][0] != "application/json; charset=utf-8" {
		t.Fatalf("list response = %+v", response)
	}
	var listing appWorkspace.Listing
	if err := json.Unmarshal(response.Body, &listing); err != nil {
		t.Fatal(err)
	}
	if listing.Path != "" || listing.Entries == nil {
		t.Fatalf("listing = %+v", listing)
	}

	response, err = backend.Handle(request(root, http.MethodGet, "files/search", map[string][]string{"q": {"app"}}))
	if err != nil {
		t.Fatal(err)
	}
	var result appWorkspace.SearchResult
	if err := json.Unmarshal(response.Body, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 1 || result.Entries[0].Path != "src/app.go" {
		t.Fatalf("search result = %+v", result)
	}
}

func TestFileAndMediaResponsesPreserveDispositionAndPolicy(t *testing.T) {
	backend, root := testBackend(t)
	response, err := backend.Handle(request(root, http.MethodGet, "files/download", map[string][]string{"path": {"src/app.go"}}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusOK || string(response.Body) != "package main" {
		t.Fatalf("download = %d %q", response.Status, response.Body)
	}
	if disposition := response.Headers["Content-Disposition"][0]; !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, "app.go") {
		t.Fatalf("download disposition = %q", disposition)
	}

	response, err = backend.Handle(request(root, http.MethodGet, "files/media", map[string][]string{"path": {"pixel.png"}}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusOK || response.Headers["Content-Type"][0] != "image/png" ||
		!strings.Contains(response.Headers["Content-Disposition"][0], "inline") ||
		response.Headers["Content-Security-Policy"][0] == "" {
		t.Fatalf("media response = %+v", response)
	}

	response, err = backend.Handle(request(root, http.MethodGet, "files/media", map[string][]string{"path": {"src/app.go"}}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusUnsupportedMediaType {
		t.Fatalf("unsupported media status = %d", response.Status)
	}
}

func TestFolderDownloadReturnsNamedZip(t *testing.T) {
	backend, root := testBackend(t)
	response, err := backend.Handle(request(root, http.MethodGet, "files/download-folder", map[string][]string{"path": {"src"}}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusOK || response.Headers["Content-Type"][0] != "application/zip" ||
		!strings.Contains(response.Headers["Content-Disposition"][0], "src.zip") {
		t.Fatalf("archive response = status %d headers %v body %s", response.Status, response.Headers, response.Body)
	}
	archive, err := zip.NewReader(bytes.NewReader(response.Body), int64(len(response.Body)))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != 1 || archive.File[0].Name != "app.go" {
		t.Fatalf("archive files = %+v", archive.File)
	}
}

func TestFolderDownloadMapsSpoolLimitBeforeSuccess(t *testing.T) {
	backend, root := testBackend(t)
	spooler, err := appWorkspace.NewSpooler(t.TempDir(), 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	backend.mu.Lock()
	backend.spooler = spooler
	backend.mu.Unlock()
	response, err := backend.Handle(request(root, http.MethodGet, "files/download-folder", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d body=%s", response.Status, response.Body)
	}
}

func TestRouterPreservesMethodAndNotFoundErrors(t *testing.T) {
	backend, root := testBackend(t)
	response, _ := backend.Handle(request(root, http.MethodPost, "files", nil))
	if response.Status != http.StatusMethodNotAllowed {
		t.Fatalf("POST files status = %d", response.Status)
	}
	response, _ = backend.Handle(request(root, http.MethodGet, "missing", nil))
	if response.Status != http.StatusNotFound {
		t.Fatalf("missing status = %d", response.Status)
	}
}
