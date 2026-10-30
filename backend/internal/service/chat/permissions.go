package chat

import (
	"context"
	permission "github.com/futrx-com/remote.futrx.com/internal/rbac"
)

const (
	PermissionHostCreate    permission.Key = "chats.host.create"
	PermissionProjectCreate permission.Key = "chats.project.create"
)

func PermissionDefinitions() []permission.Definition {
	return []permission.Definition{
		{Key: PermissionHostCreate, Description: "Create or fork a host chat.", Scopes: []permission.ScopeKind{permission.ScopePlatform}, Baseline: permission.BaselineAuthenticated, Delegable: true},
		{Key: PermissionProjectCreate, Description: "Create or fork a project chat.", Scopes: []permission.ScopeKind{permission.ScopeProject}, Baseline: permission.BaselineProjectMember, Delegable: true},
	}
}

type Authorizer interface {
	Require(context.Context, permission.Check) error
}

func WithAuthorizer(authorizer Authorizer) Option {
	return func(s *Service) {
		if authorizer != nil {
			s.authorizer = authorizer
		}
	}
}

type systemOnlyAuthorizer struct{}

func (systemOnlyAuthorizer) Require(ctx context.Context, _ permission.Check) error {
	actor, ok := permission.ActorFromContext(ctx)
	if !ok {
		return permission.ErrActorRequired
	}
	if !actor.IsSystem() {
		return permission.ErrDenied
	}
	return nil
}
func (s *Service) requireCreate(ctx context.Context, projectID ProjectID) error {
	check := permission.Check{Permission: PermissionHostCreate, Scope: permission.PlatformScope()}
	if projectID != "" {
		check = permission.Check{Permission: PermissionProjectCreate, Scope: permission.ProjectScope(string(projectID))}
	}
	return s.authorizer.Require(ctx, check)
}
