package httphandlers

import (
	"context"
	"net/http"
	"path"
	"strings"

	serviceapplications "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// visibleProjects is the one project-policy capability needed to scope a
// caller's installed UI extensions.
type visibleProjects interface {
	ListVisible(ctx context.Context, email string, isAdmin bool) ([]serviceproject.Meta, error)
}

func (h *ApplicationsHandler) serveUIImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !h.requireRegistered(w, r) {
		return
	}
	if h.apps == nil {
		httptransport.SendJSON(w, http.StatusOK, []serviceapplications.UIExtension{})
		return
	}

	projectIDs, err := h.visibleProjectIDs(r)
	if err != nil {
		sendAppError(w, err)
		return
	}
	extensions, err := h.apps.UIExtensions(r.Context(), projectIDs)
	if err != nil {
		sendAppError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, extensions)
}

// visibleProjectIDs lists the projects whose installed apps count for this
// caller. Missing dependencies safely restrict the result to global installs.
func (h *ApplicationsHandler) visibleProjectIDs(r *http.Request) ([]string, error) {
	if h.projects == nil || h.auth == nil {
		return nil, nil
	}
	email, err := callerEmailFromRequest(r, h.auth)
	if err != nil || email == "" {
		return nil, nil
	}
	isAdmin, _ := h.auth.IsAdmin(r.Context(), email)
	projects, err := h.projects.ListVisible(r.Context(), email, isAdmin)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, string(project.ID))
	}
	return ids, nil
}

func (h *ApplicationsHandler) serveUIAsset(w http.ResponseWriter, r *http.Request, rest string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !h.requireRegistered(w, r) {
		return
	}
	if h.apps == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "applications unavailable")
		return
	}
	imageID, assetPath, ok := strings.Cut(rest, "/ui/")
	if !ok || strings.TrimSpace(imageID) == "" {
		httptransport.SendErr(w, http.StatusNotFound, "asset not found")
		return
	}
	data, ok := h.apps.UIAsset(imageID, assetPath)
	if !ok {
		httptransport.SendErr(w, http.StatusNotFound, "asset not found")
		return
	}
	w.Header().Set("Content-Type", uiAssetContentType(assetPath))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

// Unknown extensions remain non-executable instead of relying on sniffing.
func uiAssetContentType(assetPath string) string {
	switch strings.ToLower(path.Ext(assetPath)) {
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".html":
		return "text/html; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".woff2":
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}
