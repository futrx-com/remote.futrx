package auth

import (
	"context"
	"errors"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspaceaccess"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspaceide"

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
	workspaceAuthorizer workspaceaccess.Authorizer
	auth                *Service
	projects            ProjectAccess
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

func (v *AccessVerifier) WithWorkspaceAuthorizer(a workspaceaccess.Authorizer) *AccessVerifier {
	v.workspaceAuthorizer = a
	return v
}

// VerifyWorkspace repeats authentication and membership at the preview proxy
// boundary, then evaluates the current policy with the validated session actor.
func (v *AccessVerifier) VerifyWorkspace(ctx context.Context, cookie, slug, capability string) error {
	if err := v.Verify(ctx, cookie, slug); err != nil {
		return err
	}
	session, err := v.auth.CurrentSession(ctx, cookie)
	if err != nil || session == nil {
		return ErrAuthenticationRequired
	}
	registered, err := v.auth.IsRegistered(ctx, session.Email)
	if err != nil {
		return err
	}
	if !registered {
		return ErrAccountNotAuthorized
	}
	if v.projects == nil || slug == "" {
		return ErrProjectNotFound
	}
	project, err := v.projects.GetBySlug(ctx, slug)
	if err != nil {
		return ErrProjectNotFound
	}
	ctx = rbac.ContextWithActor(ctx, rbac.UserActor(session.Email))
	if capability == "ide" {
		return workspaceide.RequireAccess(ctx, v.workspaceAuthorizer, string(project.ID))
	}
	return workspaceaccess.Require(ctx, v.workspaceAuthorizer, capability, string(project.ID))
}
