package httphandlers

import (
	"net/http"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

type authSessionHandler struct {
	auth  *serviceauth.Service
	audit serviceaudit.Recorder
}

func (h *authSessionHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/auth/logout", h.logout)
	mux.HandleFunc("/auth/me", h.me)
}

func (h *authSessionHandler) logout(w http.ResponseWriter, r *http.Request) {
	// Read the session before the cookie is cleared, so the entry names who
	// signed out rather than an anonymous request.
	entry := serviceaudit.Success(
		serviceaudit.ActionAuthLogout,
		serviceaudit.Target{Type: serviceaudit.TargetSession},
		nil,
	)
	if session, err := h.auth.CurrentSession(httptransport.SessionCookieValue(r)); err == nil && session != nil {
		entry.Actor = serviceaudit.Actor{Email: session.Email, Sub: session.Sub}
	}
	if h.audit != nil {
		h.audit.Record(r.Context(), entry)
	}
	http.SetCookie(w, &http.Cookie{
		Name: serviceauth.SessionCookieName, Path: "/", Domain: h.auth.CookieDomain(), MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: serviceauth.SessionCookieName, Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *authSessionHandler) me(w http.ResponseWriter, r *http.Request) {
	httptransport.SendJSON(w, http.StatusOK, h.auth.Status(r.Context(), httptransport.SessionCookieValue(r)))
}
