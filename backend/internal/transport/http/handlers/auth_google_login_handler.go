package httphandlers

import (
	"context"
	crand "crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

type googleLoginHandler struct {
	auth *serviceauth.Service
}

func (h *googleLoginHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/auth/google/login", h.login)
	mux.HandleFunc("/auth/google/callback", h.callback)
}

func (h *googleLoginHandler) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	stateBytes := make([]byte, 16)
	if _, err := crand.Read(stateBytes); err != nil {
		http.Error(w, "rand", http.StatusInternalServerError)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	http.SetCookie(w, &http.Cookie{
		Name: serviceauth.StateCookieName, Value: state,
		Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
		MaxAge: 600,
	})

	if returnTo := r.URL.Query().Get("return_to"); returnTo != "" && isSafeReturnTo(returnTo, h.auth.BaseURL()) {
		http.SetCookie(w, &http.Cookie{
			Name: returnToCookieName, Value: returnTo,
			Path: "/", Domain: h.auth.CookieDomain(),
			HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
			MaxAge: 600,
		})
	}

	authURL, err := h.auth.AuthCodeURL(state)
	if err != nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *googleLoginHandler) callback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(serviceauth.StateCookieName)
	if err != nil || r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "bad oauth state - try logging in again", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: serviceauth.StateCookieName, Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := h.auth.CompleteGoogleLogin(ctx, code, localClientIP(r), r.UserAgent())
	if err != nil {
		var notInvited serviceauth.NotInvitedError
		if errors.As(err, &notInvited) {
			loginURL := h.auth.BaseURL() + "/?error=not-invited&email=" + url.QueryEscape(notInvited.Email)
			http.Redirect(w, r, loginURL, http.StatusFound)
			return
		}
		if errors.Is(err, serviceauth.ErrLocalAdminPasswordOnly) {
			http.Redirect(w, r, h.auth.BaseURL()+"/?error=admin-password", http.StatusFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !result.Completed {
		setPendingCookie(w, h.auth, result.PendingToken)
		http.Redirect(w, r, h.auth.BaseURL()+"/?twoFactorRequired=1", http.StatusFound)
		return
	}

	setSessionCookie(w, h.auth, result.CookieValue)

	target := "/"
	if cookie, err := r.Cookie(returnToCookieName); err == nil && isSafeReturnTo(cookie.Value, h.auth.BaseURL()) {
		target = cookie.Value
	}
	http.SetCookie(w, &http.Cookie{
		Name: returnToCookieName, Path: "/", Domain: h.auth.CookieDomain(), MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, target, http.StatusFound)
}
