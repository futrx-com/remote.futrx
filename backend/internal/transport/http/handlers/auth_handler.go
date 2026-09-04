package httphandlers

import (
	"net/http"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

// AuthHandler composes the independent authentication HTTP flows behind the
// RouteRegistrar expected by the transport composition root.
type AuthHandler struct {
	googleLogin  *googleLoginHandler
	local        *localAuthHandler
	twoFactor    *authTwoFactorHandler
	session      *authSessionHandler
	verify       *authVerifyHandler
	googleConfig *googleConfigHandler
}

func NewAuthHandler(auth *serviceauth.Service, access *serviceauth.AccessVerifier) *AuthHandler {
	loginLimiter := newLocalLoginLimiter()
	return &AuthHandler{
		googleLogin:  &googleLoginHandler{auth: auth},
		local:        &localAuthHandler{auth: auth, logins: loginLimiter},
		twoFactor:    &authTwoFactorHandler{auth: auth, limiter: loginLimiter},
		session:      &authSessionHandler{auth: auth},
		verify:       &authVerifyHandler{auth: auth, access: access},
		googleConfig: &googleConfigHandler{auth: auth},
	}
}

func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
	h.googleLogin.RegisterRoutes(mux)
	h.local.RegisterRoutes(mux)
	h.twoFactor.RegisterRoutes(mux)
	h.session.RegisterRoutes(mux)
	h.verify.RegisterRoutes(mux)
	h.googleConfig.RegisterRoutes(mux)
}
