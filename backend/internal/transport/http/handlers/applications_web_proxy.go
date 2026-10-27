package httphandlers

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

func newWebProxy(upstream *url.URL, scheme string) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream)
			pr.Out.Host = pr.In.Host
			// The gateway has already authenticated the caller. Container services
			// must never receive platform cookies or authorization credentials.
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Authorization")
			pr.SetXForwarded()
			pr.Out.Header.Set("X-Forwarded-Proto", scheme)
		},
		Transport: &http.Transport{DisableKeepAlives: true, Proxy: nil},
		ModifyResponse: func(response *http.Response) error {
			response.Header.Del("Set-Cookie")
			response.Header.Del("Clear-Site-Data")
			response.Header.Set("Cache-Control", "private, no-store")
			response.Header.Set("Origin-Agent-Cluster", "?1")
			// Paths, redirects, and service workers stay on the installation's
			// own origin; no HTML or path-prefix rewriting is needed.
			return nil
		},
	}
}
