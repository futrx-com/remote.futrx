package httphandlers

import (
	"errors"
	"net/http"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceemail "github.com/futrx-com/remote.futrx.com/internal/service/email"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// EmailSettingsHandler exposes the admin-only surface for configuring,
// testing, and clearing the server's single Gmail SMTP credential.
type EmailSettingsHandler struct {
	email *serviceemail.Service
	auth  *serviceauth.Service
}

func NewEmailSettingsHandler(email *serviceemail.Service, auth *serviceauth.Service) *EmailSettingsHandler {
	return &EmailSettingsHandler{email: email, auth: auth}
}

func (h *EmailSettingsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/email", h.handleSettings)
	mux.HandleFunc("/api/admin/email/test", h.handleTest)
}

func (h *EmailSettingsHandler) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
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

type emailSettingsResponse struct {
	Configured bool   `json:"configured"`
	Address    string `json:"address"`
}

func (h *EmailSettingsHandler) handleSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		settings, err := h.email.Settings(r.Context())
		if err != nil {
			sendEmailError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, emailSettingsResponse{
			Configured: settings.Configured,
			Address:    settings.Address,
		})

	case http.MethodPut:
		var body struct {
			Address     string `json:"address"`
			AppPassword string `json:"appPassword"`
		}
		if err := readJSONBody(r, &body); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid request")
			return
		}
		settings, err := h.email.Configure(r.Context(), serviceemail.Credentials{
			Address:     body.Address,
			AppPassword: body.AppPassword,
		})
		if err != nil {
			sendEmailError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, emailSettingsResponse{
			Configured: settings.Configured,
			Address:    settings.Address,
		})

	case http.MethodDelete:
		if err := h.email.Disable(r.Context()); err != nil && !errors.Is(err, serviceemail.ErrNotConfigured) {
			sendEmailError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *EmailSettingsHandler) handleTest(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var body struct {
		To string `json:"to"`
	}
	if err := readJSONBody(r, &body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.email.SendTest(r.Context(), body.To); err != nil {
		sendEmailError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, map[string]bool{"sent": true})
}

func sendEmailError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, serviceemail.ErrInvalidAddress),
		errors.Is(err, serviceemail.ErrInvalidAppPassword),
		errors.Is(err, serviceemail.ErrInvalidRecipient):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, serviceemail.ErrNotConfigured):
		httptransport.SendErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, serviceemail.ErrVerificationFailed), errors.Is(err, serviceemail.ErrSendFailed):
		httptransport.SendErr(w, http.StatusBadGateway, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
