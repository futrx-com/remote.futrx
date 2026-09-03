package httpmiddleware

import (
	"net/http"
	"strings"

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

// RequireProviderLogin keeps application APIs closed until the configured
// agent-catalog readiness policy succeeds. Agent auth/status routes remain
// open so an authenticated administrator can complete onboarding; readiness
// may also come from a catalog-declared no-auth gate module.
func (m *Auth) RequireProviderLogin(authenticated func() bool, allowedPrefixes ...string) *Auth {
	m.providerAuthenticated = authenticated
	m.providerAuthPrefixes = append([]string(nil), allowedPrefixes...)
	return m
}

func (m *Auth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/ws") {
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
		registered, _ := m.auth.IsRegistered(r.Context(), session.Email)
		if !registered {
			http.Error(w, "account not authorized", http.StatusUnauthorized)
			return
		}
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
