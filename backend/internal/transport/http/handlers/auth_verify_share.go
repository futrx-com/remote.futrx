package httphandlers

import (
	"context"
	"net/http"
	"net/url"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceshare "github.com/futrx-com/remote.futrx.com/internal/service/share"
)

// shareQueryParam carries a public preview token on the visitor's first hit.
const shareQueryParam = "share"

// shareAuthorizer is the slice of the share service this edge gate needs.
type shareAuthorizer interface {
	Validate(ctx context.Context, slug string, port int, token string) (serviceshare.Share, bool)
	Allows(ctx context.Context, slug string, port int, id serviceshare.ID) bool
}

// authorizeShare is the public-link gate for preview hosts. It reports whether
// it produced a response; when it returns false the caller falls back to the
// ordinary platform-session check.
//
// Two shapes of request arrive here:
//
//  1. First visit, carrying ?share=<token>. The token is validated for exactly
//     this slug and port, then exchanged for a cookie and a redirect to the
//     same URL without the token.
//  2. Every later request, carrying the cookie. The signed pass is checked
//     against this slug and port, and the link is re-read from the store so a
//     revoked link stops working on the next request.
//
// The exchange has to be a redirect rather than a plain 200: Caddy's
// forward_auth discards the auth response on 2xx and forwards only the
// original request, so a Set-Cookie header on a 200 would never reach the
// browser. A 3xx is relayed whole. Dropping the token from the URL also keeps
// it out of the visitor's history, the Referer header, and the project's own
// access logs.
func (h *authVerifyHandler) authorizeShare(
	w http.ResponseWriter,
	r *http.Request,
	slug string,
	port int,
) bool {
	if h.shares == nil || h.auth == nil {
		return false
	}
	// Guards the agent browser's noVNC port and anything outside the preview
	// port range; the service refuses these too.
	if err := serviceshare.ShareablePort(port); err != nil {
		return false
	}

	original := forwardedURI(r)
	if original != nil {
		if token := original.Query().Get(shareQueryParam); token != "" {
			granted, ok := h.shares.Validate(r.Context(), slug, port, token)
			if !ok {
				return false
			}
			landing := shareLandingURL(r, original, h.auth.BaseURL())
			if landing == "" {
				return false
			}
			h.startShareSession(w, r, slug, port, granted, landing)
			return true
		}
	}

	pass, err := h.auth.VerifySharePass(shareCookieValue(r))
	if err != nil || pass == nil || pass.Slug != slug || pass.Port != port {
		return false
	}
	if !h.shares.Allows(r.Context(), slug, port, serviceshare.ID(pass.ShareID)) {
		return false
	}
	w.WriteHeader(http.StatusOK)
	return true
}

func (h *authVerifyHandler) startShareSession(
	w http.ResponseWriter,
	r *http.Request,
	slug string,
	port int,
	granted serviceshare.Share,
	landing string,
) {
	expires := time.UnixMilli(granted.ExpiresAt)
	http.SetCookie(w, &http.Cookie{
		Name: serviceauth.ShareCookieName,
		Value: h.auth.SignSharePass(serviceauth.SharePass{
			Slug:    slug,
			Port:    port,
			ShareID: string(granted.ID),
			Exp:     expires.Unix(),
		}),
		// No Domain: the browser scopes the cookie to this exact preview
		// hostname, so it never reaches the base domain, the IDE, or a
		// different project's preview.
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
	http.Redirect(w, r, landing, http.StatusFound)
}

// shareLandingURL rebuilds the visitor's original URL with the token removed.
// It returns "" when the result would leave the platform's own domains, which
// the preview host pattern already rules out but is cheap to re-check.
func shareLandingURL(r *http.Request, original *url.URL, base string) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "https"
	}

	clean := *original
	query := clean.Query()
	query.Del(shareQueryParam)
	clean.RawQuery = query.Encode()
	if clean.Path == "" {
		clean.Path = "/"
	}

	landing := proto + "://" + r.Header.Get("X-Forwarded-Host") + clean.RequestURI()
	if !isSafeReturnTo(landing, base) {
		return ""
	}
	return landing
}

func forwardedURI(r *http.Request) *url.URL {
	raw := r.Header.Get("X-Forwarded-Uri")
	if raw == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return nil
	}
	return parsed
}

func shareCookieValue(r *http.Request) string {
	cookie, err := r.Cookie(serviceauth.ShareCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
