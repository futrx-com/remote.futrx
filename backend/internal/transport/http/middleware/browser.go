package httpmiddleware

import (
	"net/http"
	"net/url"
	"strings"

	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// BrowserProtection separates browser origins even when sibling subdomains
// share a SameSite cookie. It also covers GET APIs and WebSocket handshakes;
// protecting only unsafe HTTP verbs would leave those entry points exposed.
type BrowserProtection struct {
	scheme string
	host   string
}

func NewBrowserProtection(baseURL string) *BrowserProtection {
	base, _ := url.Parse(baseURL)
	if base == nil {
		base = &url.URL{}
	}
	return &BrowserProtection{scheme: base.Scheme, host: base.Host}
}

func (p *BrowserProtection) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		w.Header().Add("Vary", "Sec-Fetch-Site")
		if !p.allowed(r) {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "cross-origin request forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Origin-Agent-Cluster", "?1")
		if strings.EqualFold(r.Host, p.host) {
			// Prevent an application from embedding the platform as a same-site
			// clickjacking target or retaining a privileged opener window.
			w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		}
		next.ServeHTTP(w, r)
	})
}

func (p *BrowserProtection) allowed(r *http.Request) bool {
	if origins := r.Header.Values("Origin"); len(origins) != 0 {
		if len(origins) != 1 {
			return false
		}
		origin, err := url.Parse(origins[0])
		if err != nil || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" ||
			origin.Path != "" || !strings.EqualFold(origin.Host, r.Host) {
			return false
		}
		scheme := p.scheme
		if scheme == "" {
			scheme = "http"
			if r.TLS != nil {
				scheme = "https"
			}
		}
		if origin.Scheme != scheme {
			return false
		}
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "none", "same-origin":
		// Non-browser clients need not supply Origin or Fetch Metadata.
		return true
	default:
		// Browser links/login redirects can navigate to the platform shell or
		// an app. Cross-origin fetches, forms, embeds and socket upgrades cannot
		// reach platform APIs. OAuth callbacks still verify their state cookie.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			return false
		}
		if r.Header.Get("Upgrade") != "" || r.Header.Get("Sec-Fetch-Mode") != "navigate" {
			return false
		}
		if httptransport.IsApplicationHost(r.Host, p.host) {
			return true
		}
		// Caddy's forward_auth request inherits the original navigation's
		// metadata. The inspector is likewise a platform-served preview page.
		if r.URL.Path == "/auth/verify" || r.URL.Path == "/__remote_inspector" {
			return true
		}
		if r.Header.Get("Sec-Fetch-Dest") != "document" {
			return false
		}
		return r.URL.Path == "/" || r.URL.Path == "/auth/google/login" ||
			r.URL.Path == "/auth/google/callback" || strings.HasPrefix(r.URL.Path, "/apps/")
	}
}
