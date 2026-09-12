package httphandlers

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

var projectVerifyHostPattern = regexp.MustCompile(`^([a-z0-9][a-z0-9-]*)--(\d{4,5})\.dev\.(.+)$`)

type authVerifyHandler struct {
	auth   *serviceauth.Service
	access *serviceauth.AccessVerifier
	shares shareAuthorizer
}

func (h *authVerifyHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/auth/verify", h.verify)
}

func (h *authVerifyHandler) verify(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Host")))
	matchedSlug, matchedPort := h.matchPreviewHost(host)

	// Only the preview host class can be authorized by a public share link.
	// The IDE hosts and the main application never reach this branch, because
	// matchPreviewHost leaves the slug empty for them. It runs before the
	// session check so that a member who opens a share URL themselves also
	// gets the token stripped from it rather than forwarding it into the
	// project's own request logs.
	if matchedSlug != "" && h.authorizeShare(w, r, matchedSlug, matchedPort) {
		return
	}

	err := h.access.Verify(r.Context(), httptransport.SessionCookieValue(r), matchedSlug)
	if err == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch {
	case errors.Is(err, serviceauth.ErrAuthenticationRequired):
		h.redirectToLogin(w, r)
	case errors.Is(err, serviceauth.ErrProjectNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, serviceauth.ErrProjectAccessDenied),
		errors.Is(err, serviceauth.ErrAccountNotAuthorized):
		http.Error(w, err.Error(), http.StatusForbidden)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// matchPreviewHost resolves a forwarded host to the project slug and port
// behind a <slug>--<port>.dev.<base> preview URL. Anything else yields an
// empty slug, which keeps the caller on the session-only path.
func (h *authVerifyHandler) matchPreviewHost(host string) (string, int) {
	match := projectVerifyHostPattern.FindStringSubmatch(host)
	if match == nil {
		return "", 0
	}
	base := strings.ToLower(strings.TrimSpace(baseHost(h.auth.BaseURL())))
	if base == "" || match[3] != base {
		return "", 0
	}
	port, err := strconv.Atoi(match[2])
	if err != nil {
		return match[1], 0
	}
	return match[1], port
}

func (h *authVerifyHandler) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	base := h.auth.BaseURL()
	if base == "" {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	loginURL := base + "/"
	if returnTo := reconstructOriginalURL(r); returnTo != "" && isSafeReturnTo(returnTo, base) {
		loginURL += "?return_to=" + url.QueryEscape(returnTo)
	}
	http.Redirect(w, r, loginURL, http.StatusFound)
}
