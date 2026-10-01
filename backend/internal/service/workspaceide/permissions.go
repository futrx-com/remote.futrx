package workspaceide

import (
	"context"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
)

const (
	PermissionProjectOpen rbac.Key = "workspace.ide.open"
	PermissionHostOpen    rbac.Key = "workspace.hostide.open"
)

func PermissionDefinitions() []rbac.Definition {
	return []rbac.Definition{
		{Key: PermissionProjectOpen, Description: "Open the project's IDE.", Scopes: []rbac.ScopeKind{rbac.ScopeProject}, Baseline: rbac.BaselineProjectMember, Delegable: true},
		{Key: PermissionHostOpen, Description: "Open files from a host chat in the IDE.", Scopes: []rbac.ScopeKind{rbac.ScopePlatform}, Baseline: rbac.BaselineAuthenticated, Delegable: true},
	}
}

type Authorizer interface {
	Require(context.Context, rbac.Check) error
}
type Option func(*Service)

func WithAuthorizer(authorizer Authorizer) Option {
	return func(s *Service) {
		if authorizer != nil {
			s.authorizer = authorizer
		}
	}
}

// RequireAccess is shared by the IDE service and authenticated edge verifier.
// No authorizer is fail-closed except for explicitly trusted system callers.
func RequireAccess(ctx context.Context, authorizer Authorizer, projectID string) error {
	if authorizer == nil {
		actor, ok := rbac.ActorFromContext(ctx)
		if !ok {
			return rbac.ErrActorRequired
		}
		if actor.IsSystem() {
			return nil
		}
		return rbac.ErrDenied
	}
	check := rbac.Check{Permission: PermissionHostOpen, Scope: rbac.PlatformScope()}
	if projectID != "" {
		check = rbac.Check{Permission: PermissionProjectOpen, Scope: rbac.ProjectScope(projectID)}
	}
	return authorizer.Require(ctx, check)
}
