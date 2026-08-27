package httphandlers

import (
	"errors"
	"net/http"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceselfupdate "github.com/futrx-com/remote.futrx.com/internal/service/selfupdate"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// SelfUpdateHandler exposes the admin-only release update flow: read the
// current status, check origin for newer release tags, and start the safe
// application or infrastructure path toward one.
type SelfUpdateHandler struct {
	updates *serviceselfupdate.Service
	auth    *serviceauth.Service
}

func NewSelfUpdateHandler(updates *serviceselfupdate.Service, auth *serviceauth.Service) *SelfUpdateHandler {
	return &SelfUpdateHandler{updates: updates, auth: auth}
}

func (h *SelfUpdateHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/update/status", h.handleStatus)
	mux.HandleFunc("/api/admin/update/check", h.handleCheck)
	mux.HandleFunc("/api/admin/update/apply", h.handleApply)
}

func (h *SelfUpdateHandler) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	if h.auth == nil {
		httptransport.SendErr(w, http.StatusForbidden, "admin only")
		return "", false
	}
	email, err := callerEmailFromRequest(r, h.auth)
	if err != nil || email == "" {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return "", false
	}
	if admin, _ := h.auth.IsAdmin(r.Context(), email); !admin {
		httptransport.SendErr(w, http.StatusForbidden, "admin only")
		return "", false
	}
	return email, true
}

func (h *SelfUpdateHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	httptransport.SendJSON(w, http.StatusOK, h.updates.Status(r.Context()))
}

func (h *SelfUpdateHandler) handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	httptransport.SendJSON(w, http.StatusOK, h.updates.Check(r.Context()))
}

func (h *SelfUpdateHandler) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	email, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Tag string `json:"tag"`
	}
	if r.ContentLength != 0 {
		if err := readJSONBody(r, &body); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid request")
			return
		}
	}
	status, err := h.updates.Apply(r.Context(), email, body.Tag)
	switch {
	case errors.Is(err, serviceselfupdate.ErrUpdateInProgress):
		httptransport.SendErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, serviceselfupdate.ErrNoReleaseTag),
		errors.Is(err, serviceselfupdate.ErrUnknownTag):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	case err != nil:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	default:
		httptransport.SendJSON(w, http.StatusOK, status)
	}
}
