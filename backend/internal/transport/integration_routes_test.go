package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestIntegratedCompositionPreservesRoutesAndRemovesHostShell(t *testing.T) {
	h, err := NewHTTPHandler(Dependencies{Static: fstest.MapFS{}, DataDir: t.TempDir(), PublicHostname: "remote.test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/sessions", "/api/sessions/create", "/api/sessions/test/send", "/ws", "/ws/terminal", "/api/schedules"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			r := httptest.NewRequest(method, "https://remote.test"+path, nil)
			r.Header.Set("Origin", "https://remote.test")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 404 && w.Code != 405 {
				t.Fatalf("removed %s %s: %d %s", method, path, w.Code, w.Body)
			}
		}
	}
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/admin/email", 401}, {"/agent-api/applications/scheduled-tasks/tasks", 503},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "https://remote.test"+tc.path, nil))
		if w.Code != tc.want {
			t.Fatalf("supported %s: %d %s", tc.path, w.Code, w.Body)
		}
	}
}
