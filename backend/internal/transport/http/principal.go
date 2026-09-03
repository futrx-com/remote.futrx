package httptransport

import (
	"context"
	"errors"
	"net/http"
	"strings"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

// PrincipalResolver centralizes the translation from an HTTP request to the
// authenticated application principal used by handlers and access gates.
type PrincipalResolver struct {
	auth *serviceauth.Service
}

func NewPrincipalResolver(auth *serviceauth.Service) *PrincipalResolver {
	return &PrincipalResolver{auth: auth}
}

func (r *PrincipalResolver) Session(request *http.Request) (*serviceauth.Session, error) {
	if r == nil || r.auth == nil {
		return nil, errors.New("no auth service")
	}
	return r.auth.CurrentSession(request.Context(), SessionCookieValue(request))
}

func (r *PrincipalResolver) Email(request *http.Request) (string, error) {
	session, err := r.Session(request)
	if err != nil || session == nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(session.Email)), nil
}

func (r *PrincipalResolver) EmailAndAdmin(ctx context.Context, request *http.Request) (string, bool, error) {
	email, err := r.Email(request)
	if err != nil || email == "" {
		return "", false, err
	}
	isAdmin, err := r.auth.IsAdmin(ctx, email)
	if err != nil {
		return email, false, err
	}
	return email, isAdmin, nil
}

func SessionCookieValue(request *http.Request) string {
	cookie, err := request.Cookie(serviceauth.SessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
