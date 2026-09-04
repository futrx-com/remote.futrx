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

// Extension assets are embedded in the binary, so a timed cache would serve a
// stale extension for its whole lifetime after a rebuild — with no URL change
// to signal it. Revalidation is what makes an edit to ui/ visible on reload,
// so the conditional-request handling is worth pinning.
func TestUIAssetETagMatching(t *testing.T) {
	etag := uiAssetETag([]byte("console.log(1)"))
	for _, tc := range []struct {
		name   string
		header string
		want   bool
	}{
		{"exact", etag, true},
		{"wildcard", "*", true},
		{"weak validator", "W/" + etag, true},
		{"among several", `"other", ` + etag, true},
		{"padded", "  " + etag + "  ", true},
		{"absent", "", false},
		{"different asset", `"0123456789abcdef0123456789abcdef"`, false},
		{"prefix of the real one", etag[:8] + `"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := etagMatches(tc.header, etag); got != tc.want {
				t.Errorf("etagMatches(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}

// The validator has to follow the bytes, or a rebuild would keep answering 304.
func TestUIAssetETagFollowsContent(t *testing.T) {
	before := uiAssetETag([]byte("export default function () {}"))
	after := uiAssetETag([]byte("export default function () { /* edited */ }"))
	if before == after {
		t.Error("editing an asset did not change its ETag")
	}
	if before != uiAssetETag([]byte("export default function () {}")) {
		t.Error("the same bytes produced two different ETags")
	}
	if len(before) < 3 || before[0] != '"' || before[len(before)-1] != '"' {
		t.Errorf("ETag %s is not a quoted strong validator", before)
	}
}
