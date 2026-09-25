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
