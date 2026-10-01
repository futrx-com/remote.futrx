package httphandlers

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

var webRoutePart = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// WebHandler dispatches by Host before platform path routing. Even /api/,
// /auth/, and /internal/ on an app origin can only reach that app's upstream.
func (h *ApplicationsHandler) WebHandler(platform http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if httptransport.IsApplicationHost(r.Host, h.webHost) {
			h.serveWebHost(w, r)
			return
		}
		platform.ServeHTTP(w, r)
	})
}

// serveWeb is a launch link only. Application-controlled bytes are never
// served at this path on the main Remote origin.
func (h *ApplicationsHandler) serveWeb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/apps/"), "/", 3)
	if len(parts) < 2 || !webRoutePart.MatchString(parts[0]) || !webRoutePart.MatchString(parts[1]) {
		http.NotFound(w, r)
		return
	}
	projects, ok := h.webProjects(w, r)
	if !ok {
		return
	}
	for _, project := range projects {
		if project.Slug != parts[0] {
			continue
		}
		target, available, err := h.apps.ProjectWebTarget(r.Context(), string(project.ID), parts[1])
		if err != nil {
			http.Error(w, "application unavailable", http.StatusInternalServerError)
			return
		}
		host := httptransport.ApplicationHost(target.InstanceID, h.webHost)
		if !available || host == "" {
			break
		}
		// Preserve escaped slashes in the app path, without allowing the input
		// to replace the destination authority (including paths beginning //).
		escaped := strings.SplitN(strings.TrimPrefix(r.URL.EscapedPath(), "/apps/"), "/", 3)
		path := "/"
		if len(escaped) == 3 {
			path += escaped[2]
		}
		location := h.webScheme() + "://" + host + path
		if r.URL.RawQuery != "" {
			location += "?" + r.URL.RawQuery
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		http.Redirect(w, r, location, http.StatusFound)
		return
	}
	http.NotFound(w, r)
}

func (h *ApplicationsHandler) serveWebHost(w http.ResponseWriter, r *http.Request) {
	id, ok := httptransport.ApplicationInstanceID(r.Host, h.webHost)
	if !ok {
		http.NotFound(w, r)
		return
	}
	projects, ok := h.webProjects(w, r)
	if !ok {
		return
	}
	target, available, err := h.apps.WebTarget(r.Context(), id)
	if err != nil {
		http.Error(w, "application unavailable", http.StatusInternalServerError)
		return
	}
	if available {
		for _, project := range projects {
			if string(project.ID) == target.ProjectID {
				upstream := &url.URL{Scheme: "http", Host: net.JoinHostPort(project.Slug+".lxd", fmt.Sprint(target.Port))}
				proxy := newWebProxy(upstream, h.webScheme())
				if h.webTransport != nil {
					proxy.Transport = h.webTransport
				}
				proxy.ServeHTTP(w, r)
				return
			}
		}
	}
	http.NotFound(w, r)
}

func (h *ApplicationsHandler) webProjects(w http.ResponseWriter, r *http.Request) ([]serviceproject.Meta, bool) {
	if h.apps == nil || h.auth == nil || h.projects == nil || h.webHost == "" {
		http.Error(w, "application web routes unavailable", http.StatusServiceUnavailable)
		return nil, false
	}
	email, err := callerEmailFromRequest(r, h.auth)
	if err != nil || email == "" {
		if r.Method == http.MethodGet && r.Header.Get("Upgrade") == "" {
			returnTo := h.webScheme() + "://" + r.Host + r.URL.RequestURI()
			if isSafeReturnTo(returnTo, h.auth.BaseURL()) {
				w.Header().Set("Cache-Control", "no-store")
				http.Redirect(w, r, h.auth.BaseURL()+"/?return_to="+url.QueryEscape(returnTo), http.StatusFound)
				return nil, false
			}
		}
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return nil, false
	}
	registered, err := h.auth.IsRegistered(r.Context(), email)
	if err != nil || !registered {
		http.Error(w, "account not authorized", http.StatusForbidden)
		return nil, false
	}
	isAdmin, err := h.auth.IsAdmin(r.Context(), email)
	if err != nil {
		http.Error(w, "project access unavailable", http.StatusInternalServerError)
		return nil, false
	}
	projects, err := h.projects.ListVisible(r.Context(), email, isAdmin)
	if err != nil {
		http.Error(w, "project access unavailable", http.StatusInternalServerError)
		return nil, false
	}
	return projects, true
}

func (h *ApplicationsHandler) webScheme() string {
	if h.auth != nil {
		if base, err := url.Parse(h.auth.BaseURL()); err == nil && base.Scheme == "http" {
			return "http"
		}
	}
	return "https"
}
