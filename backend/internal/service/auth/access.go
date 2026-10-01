package auth

import (
	"context"
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspaceaccess"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspaceide"
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
	ideAuthorizer workspaceide.Authorizer
	auth          *Service
	projects      ProjectAccess
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

// WithIDEAuthorizer wires the same authorizer used by IDE open-file requests.
func (v *AccessVerifier) WithIDEAuthorizer(authorizer workspaceide.Authorizer) *AccessVerifier {
	v.ideAuthorizer = authorizer
	return v
}

// VerifyIDE verifies the session and project membership before checking IDE
// policy. Forward-auth bypasses API middleware, so it attaches only the actor
// resolved from the validated session here, never a forwarded identity header.
func (v *AccessVerifier) VerifyIDE(ctx context.Context, sessionCookie, projectSlug string) error {
	return v.verifyWorkspace(ctx, sessionCookie, projectSlug, false)
}

func (v *AccessVerifier) VerifyBrowser(ctx context.Context, sessionCookie, projectSlug string) error {
	return v.verifyWorkspace(ctx, sessionCookie, projectSlug, true)
}

func (v *AccessVerifier) verifyWorkspace(ctx context.Context, sessionCookie, projectSlug string, browser bool) error {
	session, err := v.auth.CurrentSession(ctx, sessionCookie)
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
	if v.projects == nil || projectSlug == "" {
		return ErrProjectNotFound
	}
	project, err := v.projects.GetBySlug(ctx, projectSlug)
	if err != nil {
		return ErrProjectNotFound
	}
	admin, err := v.auth.IsAdmin(ctx, session.Email)
	if err != nil {
		return err
	}
	if !admin {
		member, err := v.projects.HasAccess(ctx, project.ID, session.Email)
		if err != nil {
			return err
		}
		if !member {
			return ErrProjectAccessDenied
		}
	}
	ctx = rbac.ContextWithActor(ctx, rbac.UserActor(session.Email))
	if browser {
		return workspaceaccess.Require(ctx, v.ideAuthorizer, "browser", string(project.ID))
	}
	return workspaceide.RequireAccess(ctx, v.ideAuthorizer, string(project.ID))
}

func (v *AccessVerifier) ProjectForAudit(ctx context.Context, slug string) (serviceproject.Meta, error) {
	if v.projects == nil {
		return serviceproject.Meta{}, ErrProjectNotFound
	}
	return v.projects.GetBySlug(ctx, slug)
}
