package project

import (
	"context"

	permission "github.com/futrx-com/remote.futrx.com/internal/rbac"
)

// Permissions owned by the project service. Keys are stable persisted
// identifiers; renaming one needs a store migration or a declared alias.
const (
	PermissionCreate permission.Key = "projects.project.create"
	// PermissionLifecycleManage gates starting, stopping, restarting, and
	// repairing the network of a project's container.
	PermissionLifecycleManage permission.Key = "projects.lifecycle.manage"
	// PermissionAccessManage gates listing and editing a project's members.
	PermissionAccessManage permission.Key = "projects.access.manage"
)

// PermissionDefinitions declares the permissions this service enforces. The
// composition root registers them. Compatibility baselines preserve existing
// registered-user creation and project-member lifecycle/access behavior.
func PermissionDefinitions() []permission.Definition {
	return []permission.Definition{
		{Key: PermissionCreate, Description: "Create a project.", Scopes: []permission.ScopeKind{permission.ScopePlatform}, Baseline: permission.BaselineAuthenticated, Delegable: true},
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

// projectAuthorizer checks one bound permission for a project.
type projectAuthorizer func(ctx context.Context, id ID) error

// bind fixes the authorizer and permission once, leaving the context and
// project ID to each call. It applies the same ID validation as require.
func bind(authorizer Authorizer, key permission.Key) projectAuthorizer {
	return func(ctx context.Context, id ID) error {
		if !ValidID(id) {
			return ErrInvalidID
		}
		return authorizer.Require(ctx, permission.Check{
			Permission: key,
			Scope:      permission.ProjectScope(string(id)),
		})
	}
}
