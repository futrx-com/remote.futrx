package httphandlers

import (
	"net/http"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceshare "github.com/futrx-com/remote.futrx.com/internal/service/share"
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

func NewAuthHandler(auth *serviceauth.Service, access *serviceauth.AccessVerifier) *AuthHandler {
	return &AuthHandler{
		googleLogin:  &googleLoginHandler{auth: auth},
		local:        &localAuthHandler{auth: auth, logins: newLocalLoginLimiter()},
		session:      &authSessionHandler{auth: auth},
		verify:       &authVerifyHandler{auth: auth, access: access},
		googleConfig: &googleConfigHandler{auth: auth},
	}
}

// WithShares lets public preview links authorize <slug>--<port>.dev.<host>
// requests at /auth/verify. Without it the edge stays session-only.
func (h *AuthHandler) WithShares(shares *serviceshare.Service) *AuthHandler {
	if shares != nil {
		h.verify.shares = shares
	}
	return h
}

func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
	h.googleLogin.RegisterRoutes(mux)
	h.local.RegisterRoutes(mux)
	h.session.RegisterRoutes(mux)
	h.verify.RegisterRoutes(mux)
	h.googleConfig.RegisterRoutes(mux)
}
