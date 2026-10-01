package httphandlers

import (
	"errors"
	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
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
	ideSlug, ideRequest, targetErr := h.ideTarget(r, host)
	if matchedSlug != "" && (matchedPort == configconstants.ProjectPreviewIDEProxyPort || matchedPort == configconstants.ProjectPreviewIDEDirectPort) {
		ideSlug, ideRequest = matchedSlug, true
	}
	if targetErr != nil {
		http.Error(w, "invalid IDE target", http.StatusBadRequest)
		return
	}
	if !ideRequest && matchedPort != configconstants.ProjectPreviewAgentBrowserPort && matchedSlug != "" && h.authorizeShare(w, r, matchedSlug, matchedPort) {
		return
	}

	var err error
	if ideRequest {
		err = h.access.VerifyIDE(r.Context(), httptransport.SessionCookieValue(r), ideSlug)
	} else if matchedSlug != "" {
		err = h.access.VerifyBrowser(r.Context(), httptransport.SessionCookieValue(r), matchedSlug)
	} else {
		err = h.access.Verify(r.Context(), httptransport.SessionCookieValue(r), matchedSlug)
	}
	if err == nil {
		if ideRequest && r.Header.Get("Sec-Fetch-Dest") == "document" {
			entry := serviceaudit.Success(serviceaudit.ActionWorkspaceIDEOpen, serviceaudit.Target{Type: serviceaudit.TargetProject}, nil)
			if project, e := h.access.ProjectForAudit(r.Context(), ideSlug); e == nil {
				entry.Target.ID = string(project.ID)
			}
			if session, e := h.auth.CurrentSession(r.Context(), httptransport.SessionCookieValue(r)); e == nil && session != nil {
				entry.Actor.Email = session.Email
			}
			h.auth.Audit().Record(r.Context(), entry)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	switch {
	case errors.Is(err, serviceauth.ErrAuthenticationRequired):
		h.redirectToLogin(w, r)
	case errors.Is(err, serviceauth.ErrProjectNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, rbac.ErrDenied), errors.Is(err, rbac.ErrActorRequired),
		errors.Is(err, serviceauth.ErrProjectAccessDenied),
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

// ideTarget recognizes only URL forms routed to the built-in IDE by Caddy.
// Launcher assets and its API retain ordinary session/API authorization.
func (h *authVerifyHandler) ideTarget(r *http.Request, host string) (string, bool, error) {
	base := strings.ToLower(strings.TrimSpace(baseHost(h.auth.BaseURL())))
	if base == "" {
		return "", false, nil
	}
	suffix := ".code." + base
	if strings.HasSuffix(host, suffix) {
		slug := strings.TrimSuffix(host, suffix)
		if !ideSlugPattern.MatchString(slug) {
			return "", true, errors.New("invalid IDE slug")
		}
		return slug, true, nil
	}
	if host != "code."+base {
		return "", false, nil
	}
	original := forwardedURI(r)
	if original == nil || original.Path == "" || !strings.HasPrefix(original.Path, "/") {
		return "", true, errors.New("missing IDE path")
	}
	for _, segment := range strings.Split(original.Path, "/") {
		if segment == "." || segment == ".." {
			return "", true, errors.New("ambiguous IDE path")
		}
	}
	switch original.Path {
	case "/", "/index.html", "/manifest.webmanifest", "/sw.js", "/icon.svg", "/favicon.ico":
		return "", false, nil
	}
	if strings.HasPrefix(original.Path, "/api/") {
		return "", false, nil
	}
	slug := strings.SplitN(strings.TrimPrefix(original.Path, "/"), "/", 2)[0]
	if !ideSlugPattern.MatchString(slug) {
		return "", true, errors.New("invalid IDE slug")
	}
	return slug, true, nil
}

var ideSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
