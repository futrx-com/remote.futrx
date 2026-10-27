package httphandlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceusersettings "github.com/futrx-com/remote.futrx.com/internal/service/usersettings"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

type UserSettingsHandler struct {
	settings *serviceusersettings.Service
	auth     *serviceauth.Service
}

func NewUserSettingsHandler(
	settings *serviceusersettings.Service,
	auth *serviceauth.Service,
) *UserSettingsHandler {
	return &UserSettingsHandler{settings: settings, auth: auth}
}

func (h *UserSettingsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/me/settings", h.HandleSettings)
}

func (h *UserSettingsHandler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "user settings unavailable")
		return
	}

	key, err := h.key(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		settings, err := h.settings.Get(r.Context(), key)
		if err != nil {
			sendUserSettingsError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, settings)

	case http.MethodPatch:
		var input serviceusersettings.UpdateInput
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&input); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		settings, err := h.settings.Update(r.Context(), key, input)
		if err != nil {
			sendUserSettingsError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, settings)

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *UserSettingsHandler) key(r *http.Request) (serviceusersettings.Key, error) {
	if h.auth == nil {
		return serviceusersettings.LocalAdminKey, nil
	}

	session, err := httptransport.NewPrincipalResolver(h.auth).Session(r)
	if err != nil {
		return "", err
	}
	return serviceusersettings.KeyFromSession(session.Email, session.Sub)
}

func sendUserSettingsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, serviceusersettings.ErrInvalidIdentity):
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
	case errors.Is(err, serviceusersettings.ErrInvalidTheme),
		errors.Is(err, serviceusersettings.ErrInvalidChatProvider),
		errors.Is(err, serviceusersettings.ErrInvalidChatMode),
		errors.Is(err, serviceusersettings.ErrInvalidReasoningEffort),
		errors.Is(err, serviceusersettings.ErrInvalidServiceTier),
		errors.Is(err, serviceusersettings.ErrInvalidApprovalPolicy),
		errors.Is(err, serviceusersettings.ErrInvalidSandboxPolicy):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
