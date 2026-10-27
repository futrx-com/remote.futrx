package auth

import (
	"context"
	"errors"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

var (
	ErrAuthenticationRequired = errors.New("authentication required")
	ErrProjectNotFound        = errors.New("no such project")
	ErrProjectAccessDenied    = errors.New("forbidden - not a member of this project")
	ErrAccountNotAuthorized   = errors.New("account not authorized")
)

type ProjectAccess interface {
	GetBySlug(ctx context.Context, slug string) (serviceproject.Meta, error)
	HasAccess(ctx context.Context, id serviceproject.ID, email string) (bool, error)
}

type AccessVerifier struct {
	auth     *Service
	projects ProjectAccess
}

func NewAccessVerifier(auth *Service, projects ProjectAccess) *AccessVerifier {
	return &AccessVerifier{auth: auth, projects: projects}
}

func (v *AccessVerifier) Verify(ctx context.Context, sessionCookie, projectSlug string) error {
	session, sessionErr := v.auth.CurrentSession(ctx, sessionCookie)
	authenticated := sessionErr == nil && session != nil

	if projectSlug != "" && v.projects != nil {
		if !authenticated {
			return ErrAuthenticationRequired
		}
		project, err := v.projects.GetBySlug(ctx, projectSlug)
		if err != nil {
			return ErrProjectNotFound
		}
		isAdmin, _ := v.auth.IsAdmin(ctx, session.Email)
		if !isAdmin {
			hasAccess, _ := v.projects.HasAccess(ctx, project.ID, session.Email)
			if !hasAccess {
				return ErrProjectAccessDenied
			}
		}
		return nil
	}

	if !authenticated {
		return ErrAuthenticationRequired
	}
	registered, _ := v.auth.IsRegistered(ctx, session.Email)
	if !registered {
		return ErrAccountNotAuthorized
	}
	return nil
}
