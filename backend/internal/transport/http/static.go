package httptransport

import (
	"io/fs"
	"net/http"
	"strings"
)

func NewStaticHandler(static fs.FS) http.Handler {
	files := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, ".html"):
			w.Header().Set("Cache-Control", "no-cache")
		case r.URL.Path == "/sw.js":
			// The service worker is the app's update mechanism; a cached copy
			// would keep an old one installed after a deploy.
			w.Header().Set("Cache-Control", "no-cache")
		case r.URL.Path == "/manifest.webmanifest":
			// Go's mime table has no entry for .webmanifest, and the file
			// server would otherwise sniff it as text/plain.
			w.Header().Set("Content-Type", "application/manifest+json")
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
