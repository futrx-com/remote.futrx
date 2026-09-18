package project

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/service/permission"
)

// Permissions owned by the project service. Keys are stable persisted
// identifiers; renaming one needs a store migration or a declared alias.
const (
	// PermissionLifecycleManage gates starting, stopping, restarting, and
	// repairing the network of a project's container.
	PermissionLifecycleManage permission.Key = "projects.lifecycle.manage"
	// PermissionAccessManage gates listing and editing a project's members.
	PermissionAccessManage permission.Key = "projects.access.manage"
)

// PermissionDefinitions declares the permissions this service enforces. The
// composition root registers them. Both use the project-member baseline so
// members keep the access they had before these permissions existed.
func PermissionDefinitions() []permission.Definition {
	return []permission.Definition{
		{
			Key:         PermissionLifecycleManage,
			Description: "Start, stop, restart, and repair the network of a project's container.",
			Scopes:      []permission.ScopeKind{permission.ScopeProject},
			Baseline:    permission.BaselineProjectMember,
			Delegable:   true,
		},
		{
			Key:         PermissionAccessManage,
			Description: "List, add, and remove the members of a project.",
			Scopes:      []permission.ScopeKind{permission.ScopeProject},
			Baseline:    permission.BaselineProjectMember,
			Delegable:   true,
		},
	}
}

// Authorizer is the only authorization capability the project service needs.
// The service depends on the permission vocabulary, not on a concrete
// permission service or store.
type Authorizer interface {
	Require(ctx context.Context, check permission.Check) error
}

// WithAuthorizer supplies the authorizer that guards the protected entry
// points. Without one the service fails closed: only the explicit system
// actor may call them.
func WithAuthorizer(authorizer Authorizer) Option {
	return func(service *Service) {
		if authorizer != nil {
			service.authorizer = authorizer
		}
	}
}

// systemOnlyAuthorizer is the fail-closed default for a Service built without
// an authorizer.
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

// require is the one helper every protected entry point uses, so each builds
// the same check. The identifier is validated first so a malformed one keeps
// its ErrInvalidID meaning instead of surfacing as an invalid scope.
func (s *Service) require(ctx context.Context, key permission.Key, id ID) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	return s.authorizer.Require(ctx, permission.Check{
		Permission: key,
		Scope:      permission.ProjectScope(string(id)),
	})
}
