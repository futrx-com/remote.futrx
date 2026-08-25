package httphandlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	serviceapplications "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// ApplicationsHandler serves the installable-apps API. Global routes
// (/api/applications/*) are admin-only because a global app is server-wide
// infrastructure. Per-project routes are delegated here by ProjectHandler,
// which has already enforced project membership.
type ApplicationsHandler struct {
	apps *serviceapplications.Service
	auth *serviceauth.Service
}

// NewApplicationsHandler builds the handler. apps may be nil when the server
// has no container runtime; routes then report the feature unavailable.
func NewApplicationsHandler(apps *serviceapplications.Service, auth *serviceauth.Service) *ApplicationsHandler {
	return &ApplicationsHandler{apps: apps, auth: auth}
}

func (h *ApplicationsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/applications", h.handleCollection)
	mux.HandleFunc("/api/applications/", h.handleResource)
}

// installBody is the shared request shape for installing an app.
type installBody struct {
	ImageID      string            `json:"imageId"`
	Name         string            `json:"name"`
	Env          map[string]string `json:"env"`
	ExternalPort int               `json:"externalPort"`
	BindAddress  string            `json:"bindAddress"`
}

type portBody struct {
	Port int `json:"port"`
}

// ---- global routes ---------------------------------------------------------

func (h *ApplicationsHandler) handleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// List global instances (admin only).
		if !h.requireAdmin(w, r) {
			return
		}
		views, err := h.apps.ListGlobal(r.Context())
		if err != nil {
			sendAppError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, orEmpty(views))
	case http.MethodPost:
		if !h.requireAdmin(w, r) {
			return
		}
		h.install(w, r, serviceapplications.ScopeGlobal, "")
	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *ApplicationsHandler) handleResource(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/applications/")

	// /api/applications/catalog is readable by any registered user, since the
	// project UI uses the same catalog.
	if rest == "catalog" {
		if r.Method != http.MethodGet {
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
		httptransport.SendJSON(w, http.StatusOK, h.apps.Catalog())
		return
	}

	// Everything else operates on a specific global instance: admin only.
	if !h.requireAdmin(w, r) {
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	id := strings.TrimSpace(parts[0])
	if id == "" {
		httptransport.SendErr(w, http.StatusBadRequest, "missing application id")
		return
	}
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	// Global instances only: reject ids that belong to a project.
	if !h.ensureGlobal(w, r, id) {
		return
	}
	h.instanceAction(w, r, id, action)
}

// ---- project routes (delegated by ProjectHandler) --------------------------

// HandleProject serves /api/projects/{projectID}/applications[/...]. The caller
// (ProjectHandler) has already verified project membership. parts is the
// project-path split: [projectID, "applications", "<rest>"].
func (h *ApplicationsHandler) HandleProject(w http.ResponseWriter, r *http.Request, projectID string, parts []string) {
	if h.apps == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "applications unavailable")
		return
	}
	sub := ""
	if len(parts) >= 3 {
		sub = parts[2]
	}
	if sub == "" {
		switch r.Method {
		case http.MethodGet:
			views, err := h.apps.ListProject(r.Context(), projectID)
			if err != nil {
				sendAppError(w, err)
				return
			}
			httptransport.SendJSON(w, http.StatusOK, orEmpty(views))
		case http.MethodPost:
			h.install(w, r, serviceapplications.ScopeProject, projectID)
		default:
			httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	segs := strings.SplitN(sub, "/", 2)
	id := strings.TrimSpace(segs[0])
	action := ""
	if len(segs) == 2 {
		action = segs[1]
	}
	if id == "" {
		httptransport.SendErr(w, http.StatusBadRequest, "missing application id")
		return
	}
	// Ownership guard: the instance must belong to this project so a member of
	// one project cannot control another project's app by guessing its id.
	if !h.ensureProject(w, r, id, projectID) {
		return
	}
	h.instanceAction(w, r, id, action)
}

// ---- shared install + lifecycle -------------------------------------------

func (h *ApplicationsHandler) install(w http.ResponseWriter, r *http.Request, scope serviceapplications.Scope, projectID string) {
	if h.apps == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "applications unavailable")
		return
	}
	var body installBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	view, err := h.apps.Install(r.Context(), serviceapplications.InstallRequest{
		ImageID:      strings.TrimSpace(body.ImageID),
		Scope:        scope,
		ProjectID:    projectID,
		Name:         body.Name,
		Env:          body.Env,
		ExternalPort: body.ExternalPort,
		BindAddress:  body.BindAddress,
	})
	if err != nil {
		sendAppError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusCreated, view)
}

// instanceAction dispatches start/stop/port and DELETE on a resolved instance.
func (h *ApplicationsHandler) instanceAction(w http.ResponseWriter, r *http.Request, id, action string) {
	switch action {
	case "":
		switch r.Method {
		case http.MethodGet:
			view, ok, err := h.apps.Get(r.Context(), id)
			if err != nil {
				sendAppError(w, err)
				return
			}
			if !ok {
				httptransport.SendErr(w, http.StatusNotFound, "application not found")
				return
			}
			httptransport.SendJSON(w, http.StatusOK, view)
		case http.MethodDelete:
			if err := h.apps.Uninstall(r.Context(), id); err != nil {
				sendAppError(w, err)
				return
			}
			httptransport.SendJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "credentials":
		if r.Method != http.MethodGet {
			httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		creds, err := h.apps.Credentials(r.Context(), id)
		if err != nil {
			sendAppError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, creds)
	case "start":
		h.lifecycle(w, r, func() (serviceapplications.View, error) { return h.apps.Start(r.Context(), id) })
	case "stop":
		h.lifecycle(w, r, func() (serviceapplications.View, error) { return h.apps.Stop(r.Context(), id) })
	case "port":
		if r.Method != http.MethodPut {
			httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body portBody
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		view, err := h.apps.SetPort(r.Context(), id, body.Port)
		if err != nil {
			sendAppError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, view)
	default:
		httptransport.SendErr(w, http.StatusNotFound, "unknown action")
	}
}

func (h *ApplicationsHandler) lifecycle(w http.ResponseWriter, r *http.Request, fn func() (serviceapplications.View, error)) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	view, err := fn()
	if err != nil {
		sendAppError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, view)
}

// ---- guards ----------------------------------------------------------------

func (h *ApplicationsHandler) ensureGlobal(w http.ResponseWriter, r *http.Request, id string) bool {
	view, ok, err := h.apps.Get(r.Context(), id)
	if err != nil {
		sendAppError(w, err)
		return false
	}
	if !ok {
		httptransport.SendErr(w, http.StatusNotFound, "application not found")
		return false
	}
	if view.Scope != serviceapplications.ScopeGlobal {
		httptransport.SendErr(w, http.StatusNotFound, "application not found")
		return false
	}
	return true
}

func (h *ApplicationsHandler) ensureProject(w http.ResponseWriter, r *http.Request, id, projectID string) bool {
	view, ok, err := h.apps.Get(r.Context(), id)
	if err != nil {
		sendAppError(w, err)
		return false
	}
	if !ok || view.Scope != serviceapplications.ScopeProject || view.ProjectID != projectID {
		httptransport.SendErr(w, http.StatusNotFound, "application not found")
		return false
	}
	return true
}

func (h *ApplicationsHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if h.apps == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "applications unavailable")
		return false
	}
	email, err := callerEmailFromRequest(r, h.auth)
	if err != nil || email == "" {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	ok, _ := h.auth.IsAdmin(r.Context(), email)
	if !ok {
		httptransport.SendErr(w, http.StatusForbidden, "admin only")
		return false
	}
	return true
}

func (h *ApplicationsHandler) requireRegistered(w http.ResponseWriter, r *http.Request) bool {
	if h.auth == nil {
		return true
	}
	email, err := callerEmailFromRequest(r, h.auth)
	if err != nil || email == "" {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	return true
}

// ---- helpers ---------------------------------------------------------------

func orEmpty(views []serviceapplications.View) []serviceapplications.View {
	if views == nil {
		return []serviceapplications.View{}
	}
	return views
}

func sendAppError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, serviceapplications.ErrNotFound):
		httptransport.SendErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, serviceapplications.ErrAlreadyInstalled):
		httptransport.SendErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, serviceapplications.ErrUnknownImage),
		errors.Is(err, serviceapplications.ErrScope),
		errors.Is(err, serviceapplications.ErrProjectneeded),
		errors.Is(err, serviceapplications.ErrRequiredEnv),
		errors.Is(err, serviceapplications.ErrPortRange):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, serviceapplications.ErrUnavailable):
		httptransport.SendErr(w, http.StatusServiceUnavailable, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
