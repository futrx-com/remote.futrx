package httphandlers

import (
	"net/http"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

type authSessionHandler struct {
	auth *serviceauth.Service
}

func (h *authSessionHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/auth/logout", h.logout)
	mux.HandleFunc("/auth/me", h.me)
}

func (h *authSessionHandler) logout(w http.ResponseWriter, r *http.Request) {
	if session, err := h.auth.CurrentSession(r.Context(), httptransport.SessionCookieValue(r)); err == nil && session != nil {
		_ = h.auth.RevokeSession(r.Context(), session.Email)
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
