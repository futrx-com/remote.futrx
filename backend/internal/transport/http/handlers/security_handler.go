package httphandlers

import (
	"errors"
	"net/http"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// SecurityHandler exposes the authenticated account's own 2FA
// enrollment/disable, recovery-code regeneration, the three independent
// SecurityPreferences toggles, and alert acknowledgement under
// /api/me/security/*, mirroring how UserSettingsHandler owns /api/me/settings.
type SecurityHandler struct {
	auth *serviceauth.Service
}

func NewSecurityHandler(auth *serviceauth.Service) *SecurityHandler {
	return &SecurityHandler{auth: auth}
}

func (h *SecurityHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/me/security", h.handleSummary)
	mux.HandleFunc("/api/me/security/2fa/enroll", h.handleEnroll)
	mux.HandleFunc("/api/me/security/2fa/confirm", h.handleConfirm)
	mux.HandleFunc("/api/me/security/2fa/disable", h.handleDisable)
	mux.HandleFunc("/api/me/security/2fa/recovery-codes/regenerate", h.handleRegenerate)
	mux.HandleFunc("/api/me/security/preferences", h.handlePreferences)
	mux.HandleFunc("/api/me/security/alerts/ack", h.handleAckAlert)
}

func (h *SecurityHandler) session(r *http.Request) (*serviceauth.Session, error) {
	if h.auth == nil {
		return nil, errors.New("auth service unavailable")
	}
	return httptransport.NewPrincipalResolver(h.auth).Session(r)
}

func (h *SecurityHandler) requireSession(w http.ResponseWriter, r *http.Request) (*serviceauth.Session, bool) {
	session, err := h.session(r)
	if err != nil || session == nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	return session, true
}

func (h *SecurityHandler) handleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	session, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	summary, err := h.auth.SecuritySummary(r.Context(), session.Email)
	if err != nil {
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httptransport.SendJSON(w, http.StatusOK, summary)
}

func (h *SecurityHandler) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	session, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	token, secret, otpauthURL, err := h.auth.BeginTwoFactorEnrollment(r.Context(), session.Email)
	if err != nil {
		if errors.Is(err, serviceauth.ErrTwoFactorAlreadyEnabled) {
			httptransport.SendErr(w, http.StatusConflict, err.Error())
			return
		}
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httptransport.SendJSON(w, http.StatusOK, map[string]string{
		"enrollmentToken": token,
		"secret":          secret,
		"otpauthUrl":      otpauthURL,
	})
}

func (h *SecurityHandler) handleConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	session, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		EnrollmentToken string `json:"enrollmentToken"`
		Code            string `json:"code"`
	}
	if err := readJSONBody(r, &body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	result, err := h.auth.CompleteTwoFactorEnrollment(
		r.Context(),
		serviceauth.User{Email: session.Email, Sub: session.Sub},
		body.EnrollmentToken,
		body.Code,
		localClientIP(r),
		r.UserAgent(),
	)
	if err != nil {
		switch {
		case errors.Is(err, serviceauth.ErrEnrollmentTokenMismatch):
			httptransport.SendErr(w, http.StatusForbidden, err.Error())
		case errors.Is(err, serviceauth.ErrInvalidEnrollmentToken):
			httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, serviceauth.ErrInvalidTwoFactorCode):
			httptransport.SendErr(w, http.StatusUnauthorized, err.Error())
		default:
			httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if result.SessionCookieValue != "" {
		setSessionCookie(w, h.auth, result.SessionCookieValue)
	}

	httptransport.SendJSON(w, http.StatusOK, map[string]any{"recoveryCodes": result.RecoveryCodes})
}

func (h *SecurityHandler) handleDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	session, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := readJSONBody(r, &body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := h.auth.CompleteTwoFactorDisable(r.Context(), session.Email, body.Code); err != nil {
		sendTwoFactorProofError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *SecurityHandler) handleRegenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	session, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := readJSONBody(r, &body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	codes, err := h.auth.RegenerateRecoveryCodes(r.Context(), session.Email, body.Code)
	if err != nil {
		sendTwoFactorProofError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
}

func sendTwoFactorProofError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, serviceauth.ErrTwoFactorNotEnabled):
		httptransport.SendErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, serviceauth.ErrInvalidTwoFactorCode):
		httptransport.SendErr(w, http.StatusUnauthorized, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *SecurityHandler) handlePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	session, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	var update serviceauth.SecurityPreferencesUpdate
	if err := readJSONBody(r, &update); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid request")
		return
	}

	result, err := h.auth.UpdateSecurityPreferences(
		r.Context(),
		serviceauth.User{Email: session.Email, Sub: session.Sub},
		update,
		localClientIP(r),
		r.UserAgent(),
	)
	if result.SessionCookieValue != "" {
		setSessionCookie(w, h.auth, result.SessionCookieValue)
	}
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, serviceauth.ErrRecoveryCodeAlertRequiresTwoFactor) {
			status = http.StatusBadRequest
		}
		httptransport.SendErr(w, status, err.Error())
		return
	}
	httptransport.SendJSON(w, http.StatusOK, result.Summary)
}

func (h *SecurityHandler) handleAckAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	session, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	if err := h.auth.AckSecurityAlert(r.Context(), session.Email); err != nil {
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
