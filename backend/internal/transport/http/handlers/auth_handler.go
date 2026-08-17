package httphandlers

import (
	"net/http"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

// AuthHandler composes the independent authentication HTTP flows behind the
// RouteRegistrar expected by the transport composition root.
type AuthHandler struct {
	googleLogin  *googleLoginHandler
	local        *localAuthHandler
	session      *authSessionHandler
	verify       *authVerifyHandler
	googleConfig *googleConfigHandler
}

func NewAuthHandler(
	auth *serviceauth.Service,
	access *serviceauth.AccessVerifier,
	auditLog serviceaudit.Recorder,
) *AuthHandler {
	return &AuthHandler{
		googleLogin:  &googleLoginHandler{auth: auth},
		local:        &localAuthHandler{auth: auth, logins: newLocalLoginLimiter()},
		session:      &authSessionHandler{auth: auth, audit: auditLog},
		verify:       &authVerifyHandler{auth: auth, access: access},
		googleConfig: &googleConfigHandler{auth: auth},
	}
}

func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
	h.googleLogin.RegisterRoutes(mux)
	h.local.RegisterRoutes(mux)
	h.session.RegisterRoutes(mux)
	h.verify.RegisterRoutes(mux)
	h.googleConfig.RegisterRoutes(mux)
}
