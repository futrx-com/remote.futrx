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

// passwordChangeCookieName carries the signed token between a sign-in that
// proved a temporary password and the endpoint that accepts the new one.
const passwordChangeCookieName = "remote_password_change"

// passwordChangeCookiePath is the only path the browser sends the cookie to:
// the two endpoints that use it.
const passwordChangeCookiePath = "/auth/local/password"

// setPasswordChangeCookie is deliberately host-only (no Domain): the browser
// never sends it to project or application hosts, so the reverse proxy needs
// no rule to strip it, and the Path keeps it off every other endpoint of the
// platform itself.
func setPasswordChangeCookie(w http.ResponseWriter, auth *serviceauth.Service, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: passwordChangeCookieName, Value: token,
		Path:     passwordChangeCookiePath,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
		MaxAge: int(auth.PasswordChangeDuration().Seconds()),
	})
}

func clearPasswordChangeCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: passwordChangeCookieName, Path: passwordChangeCookiePath, MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

func passwordChangeCookieValue(r *http.Request) string {
	cookie, err := r.Cookie(passwordChangeCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
