package httpmiddleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserProtection(t *testing.T) {
	const platform = "remote.test"
	const app = "abcdef123456.apps.remote.test"
	for _, tc := range []struct {
		name, method, host, path, origin, site, mode, dest string
		want                                               int
	}{
		{"same origin write", "POST", platform, "/api/projects", "https://remote.test", "same-origin", "cors", "empty", 204},
		{"same origin socket", "GET", platform, "/ws/chat", "https://remote.test", "same-origin", "websocket", "empty", 204},
		{"app reads API", "GET", platform, "/api/projects", "https://" + app, "same-site", "cors", "empty", 403},
		{"app changes API", "POST", platform, "/api/projects", "https://" + app, "same-site", "cors", "empty", 403},
		{"app opens platform socket", "GET", platform, "/ws/chat", "https://" + app, "same-site", "websocket", "empty", 403},
		{"app submits form", "POST", platform, "/auth/local/login", "https://" + app, "same-site", "navigate", "document", 403},
		{"opaque origin", "POST", platform, "/api/projects", "null", "cross-site", "cors", "empty", 403},
		{"scheme mismatch", "POST", platform, "/api/projects", "http://remote.test", "same-origin", "cors", "empty", 403},
		{"deceptive origin", "POST", platform, "/api/projects", "https://remote.test.attacker.test", "same-site", "cors", "empty", 403},
		{"same-site image GET", "GET", platform, "/api/projects", "", "same-site", "no-cors", "image", 403},
		{"same-site no-cors write", "POST", platform, "/api/projects", "", "same-site", "no-cors", "empty", 403},
		{"GET form targeting API", "GET", platform, "/api/projects", "", "same-site", "navigate", "document", 403},
		{"logout navigation", "GET", platform, "/auth/logout", "", "same-site", "navigate", "document", 403},
		{"embedded platform", "GET", platform, "/", "", "same-site", "navigate", "iframe", 403},
		{"main page link", "GET", platform, "/", "", "same-site", "navigate", "document", 204},
		{"OAuth callback", "GET", platform, "/auth/google/callback", "", "cross-site", "navigate", "document", 204},
		{"launch link", "GET", platform, "/apps/project/editor/", "", "same-site", "navigate", "document", 204},
		{"application navigation", "GET", app, "/", "", "same-site", "navigate", "document", 204},
		{"embedded application", "GET", app, "/", "", "same-site", "navigate", "iframe", 204},
		{"application own API", "POST", app, "/api/save", "https://" + app, "same-origin", "cors", "empty", 204},
		{"cross application write", "POST", "123456abcdef.apps.remote.test", "/api/save", "https://" + app, "same-site", "cors", "empty", 403},
		{"preview forward auth", "GET", "project--3000.dev.remote.test", "/auth/verify", "", "same-site", "navigate", "iframe", 204},
		{"preview inspector", "GET", "project--3000.dev.remote.test", "/__remote_inspector", "", "same-site", "navigate", "iframe", 204},
		{"non-browser API client", "POST", platform, "/api/projects", "", "", "", "", 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewBrowserProtection("https://" + platform).Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(tc.method, "https://"+tc.host+tc.path, nil)
			for name, value := range map[string]string{"Origin": tc.origin, "Sec-Fetch-Site": tc.site, "Sec-Fetch-Mode": tc.mode, "Sec-Fetch-Dest": tc.dest} {
				if value != "" {
					req.Header.Set(name, value)
				}
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if rec.Code == 204 && tc.host == platform && rec.Header().Get("Content-Security-Policy") != "frame-ancestors 'self'" {
				t.Fatal("platform frame protection missing")
			}
			if rec.Code == 204 && tc.host != platform && rec.Header().Get("X-Frame-Options") != "" {
				t.Fatal("application/preview embedding was disabled")
			}
		})
	}
}
