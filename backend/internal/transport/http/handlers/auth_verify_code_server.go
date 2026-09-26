package httphandlers

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	serviceapplications "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

var codeServerPathPattern = regexp.MustCompile(`^/([a-z0-9][a-z0-9-]*)(?:/|$)`)
var projectCodePathPattern = regexp.MustCompile(`^/([a-z0-9][a-z0-9-]*)/code(?:/|$)`)

type codeServerAccess struct {
	projects     *serviceproject.Service
	applications *serviceapplications.Service
}

func (h *authVerifyHandler) verifyCodeServer(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Host")))
	matchedSlug := h.codeServerSlug(host, r.Header.Get("X-Forwarded-Uri"))
	if matchedSlug == "" {
		http.Error(w, "invalid Code Server route", http.StatusNotFound)
		return
	}
	if !h.verifySession(w, r, matchedSlug) {
		return
	}
	available, appErr := h.codeServer.available(r.Context(), matchedSlug)
	if appErr != nil {
		http.Error(w, "Code Server availability unavailable", http.StatusInternalServerError)
		return
	}
	if !available {
		http.Error(w, "Code Server is not installed or running", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *authVerifyHandler) codeServerSlug(host, forwardedURI string) string {
	base := strings.ToLower(strings.TrimSpace(baseHost(h.auth.BaseURL())))
	if base == "" {
		return ""
	}
	if host == base {
		if match := projectCodePathPattern.FindStringSubmatch(forwardedURI); match != nil {
			return match[1]
		}
		return ""
	}
	if host == "code."+base {
		if match := codeServerPathPattern.FindStringSubmatch(forwardedURI); match != nil {
			return match[1]
		}
		return ""
	}
	if suffix := ".code." + base; strings.HasSuffix(host, suffix) {
		slug := strings.TrimSuffix(host, suffix)
		if codeServerPathPattern.MatchString("/" + slug + "/") {
			return slug
		}
	}
	return ""
}

func (access *codeServerAccess) available(ctx context.Context, slug string) (bool, error) {
	if access.projects == nil || access.applications == nil {
		return false, nil
	}
	project, err := access.projects.GetBySlug(ctx, slug)
	if err != nil {
		return false, nil
	}
	installed, err := access.applications.ListProject(ctx, string(project.ID))
	if err != nil {
		return false, err
	}
	for _, app := range installed {
		if app.ApplicationID == "code-server" && app.Status == serviceapplications.StatusRunning {
			return true, nil
		}
	}
	return false, nil
}
