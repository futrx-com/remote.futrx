package httphandlers

import "testing"

// The response type is derived from the extension, never sniffed, so a catalog
// asset cannot be reinterpreted by the browser as something more dangerous
// than what its extension says.
func TestUIAssetContentType(t *testing.T) {
	for _, tc := range []struct {
		asset string
		want  string
	}{
		{"scripts/main.js", "text/javascript; charset=utf-8"},
		{"scripts/popup.MJS", "text/javascript; charset=utf-8"},
		{"style/style.css", "text/css; charset=utf-8"},
		{"views/popup.html", "text/html; charset=utf-8"},
		{"data/schema.json", "application/json; charset=utf-8"},
		{"icons/plug.svg", "image/svg+xml"},
		{"icons/logo.png", "image/png"},
		{"install.sh", "application/octet-stream"},
		{"noextension", "application/octet-stream"},
	} {
		if got := uiAssetContentType(tc.asset); got != tc.want {
			t.Errorf("uiAssetContentType(%q) = %q, want %q", tc.asset, got, tc.want)
		}
	}
}
