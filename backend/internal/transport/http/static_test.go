package httptransport

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestStaticHandlerServesAppShellForWorkspaceRoutes(t *testing.T) {
	var files fs.FS = fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("workspace app shell")},
		"assets/app.js": &fstest.MapFile{Data: []byte("app code")},
	}
	handler := NewStaticHandler(files)
	for _, path := range []string{"/settings", "/settings/notifications", "/chats/abcdef12"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.String() != "workspace app shell" {
			t.Fatalf("%s: code %d body %q", path, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"/assets/missing.js", "/api/missing", "/settings/fake.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s: code %d, want 404", path, response.Code)
		}
	}
}

func TestStaticHandlerRevalidatesFrontendBuildManifest(t *testing.T) {
	var files fs.FS = fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("app shell")},
		"build.json": &fstest.MapFile{Data: []byte(`{"build":"abc"}`)},
	}
	response := httptest.NewRecorder()
	NewStaticHandler(files).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/build.json", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"build":"abc"}` {
		t.Fatalf("code %d body %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}
