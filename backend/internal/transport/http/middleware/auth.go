package httpmiddleware

import (
	"net/http"
	"strings"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

type Auth struct {
	auth                  *serviceauth.Service
	principal             *httptransport.PrincipalResolver
	localAdminConfigured  func() bool
	providerAuthenticated func() bool
	providerAuthPrefixes  []string
}

func (m *Auth) RequireLocalAdminSetup(configured func() bool) *Auth {
	m.localAdminConfigured = configured
	return m
}

func NewAuth(auth *serviceauth.Service) *Auth {
	return &Auth{auth: auth, principal: httptransport.NewPrincipalResolver(auth)}
}

// RequireProviderLogin keeps application APIs closed until at least one AI
// provider is authenticated. Provider status and login routes remain open so
// an authenticated administrator can complete onboarding.
func (m *Auth) RequireProviderLogin(authenticated func() bool, allowedPrefixes ...string) *Auth {
	m.providerAuthenticated = authenticated
	m.providerAuthPrefixes = append([]string(nil), allowedPrefixes...)
	return m
}

func (m *Auth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/ws") {
			// Sign-in, sign-out, and the OAuth round-trip are auditable but
			// ungated. They get the network half of the caller here; their own
			// handlers supply the identity, because a login request is exactly
			// the one with no session to resolve it from.
			if strings.HasPrefix(path, "/auth/") {
				r = httptransport.WithAuditCaller(r, httptransport.AuditCallerFromRequest(r))
			}
			next.ServeHTTP(w, r)
			return
		}
		if path == "/auth/me" || strings.HasPrefix(path, "/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		session, err := m.principal.Session(r)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		// Admin implies registered, so the common admin path still costs one
		// directory lookup while every request gains the actor fields the
		// audit log needs.
		isAdmin, _ := m.auth.IsAdmin(r.Context(), session.Email)
		registered := isAdmin
		if !registered {
			registered, _ = m.auth.IsRegistered(r.Context(), session.Email)
		}
		if !registered {
			http.Error(w, "account not authorized", http.StatusUnauthorized)
			return
		}
		caller := httptransport.AuditCallerFromRequest(r)
		caller.Actor = serviceaudit.Actor{
			Email:   session.Email,
			Sub:     session.Sub,
			IsAdmin: isAdmin,
		}
		r = httptransport.WithAuditCaller(r, caller)
		if m.localAdminConfigured != nil && !m.localAdminConfigured() {
			http.Error(w, "local administrator setup required", http.StatusPreconditionRequired)
			return
		}
		if m.providerAuthenticated != nil && !m.providerAuthenticated() && !m.isProviderAuthPath(path) {
			http.Error(w, "AI provider authentication required", http.StatusPreconditionRequired)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Auth) isProviderAuthPath(path string) bool {
	for _, prefix := range m.providerAuthPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
