package service

import (
	"context"

	servicepermission "github.com/futrx-com/remote.futrx.com/internal/service/permission"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// permissionDefinitions is the complete code-owned permission catalog. Each
// owning service exports its definitions from a permissions.go file; add its
// group here so the registry validates them together.
func permissionDefinitions() [][]servicepermission.Definition {
	return [][]servicepermission.Definition{
		servicepermission.ManagementDefinitions(),
		serviceproject.PermissionDefinitions(),
	}
}

// newPermissions builds the permission service from the code-owned registry,
// the persisted policy, and narrow identity and membership adapters. The
// identity source is the auth service so the local administrator and
// directory administrators are both recognized as the root policy.
func newPermissions(
	ctx context.Context,
	repo servicepermission.Repository,
	identity servicepermission.IdentityDirectory,
	access serviceproject.AccessRepository,
) (*servicepermission.Service, error) {
	registry, err := servicepermission.NewRegistry(permissionDefinitions()...)
	if err != nil {
		return nil, err
	}
	return servicepermission.NewService(ctx, registry, repo, identity, projectMembership{access: access})
}

// projectMembership adapts the project access repository to the permission
// layer's membership port, so the evaluator does not import the project
// service.
type projectMembership struct {
	access serviceproject.AccessRepository
}

func (m projectMembership) HasAccess(ctx context.Context, projectID string, email string) (bool, error) {
	if m.access == nil {
		return false, nil
	}
	email = servicepermission.NormalizeEmail(email)
	if email == "" {
		return false, nil
	}
	return m.access.Has(ctx, serviceproject.ID(projectID), email)
}
