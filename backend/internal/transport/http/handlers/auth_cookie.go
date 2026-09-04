package httphandlers

import (
	"net/http"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

func setSessionCookie(w http.ResponseWriter, auth *serviceauth.Service, cookieValue string) {
	http.SetCookie(w, &http.Cookie{
		Name: serviceauth.SessionCookieName, Value: cookieValue,
		Path: "/", Domain: auth.CookieDomain(),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
		MaxAge: int(serviceauth.SessionDuration().Seconds()),
	})
}

func setPendingCookie(w http.ResponseWriter, auth *serviceauth.Service, pendingToken string) {
	http.SetCookie(w, &http.Cookie{
		Name: serviceauth.PendingTwoFactorCookieName, Value: pendingToken,
		Path: "/", Domain: auth.CookieDomain(),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
		MaxAge: int(auth.PendingTwoFactorDuration().Seconds()),
	})
}

func clearPendingCookie(w http.ResponseWriter, auth *serviceauth.Service) {
	http.SetCookie(w, &http.Cookie{
		Name: serviceauth.PendingTwoFactorCookieName, Path: "/", Domain: auth.CookieDomain(), MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
}

func pendingCookieValue(r *http.Request) string {
	cookie, err := r.Cookie(serviceauth.PendingTwoFactorCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
