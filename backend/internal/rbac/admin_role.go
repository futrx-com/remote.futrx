package rbac

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AdminRoleID is the stable ID of the built-in admin role. It is an ordinary
// stored role once seeded: it can be edited, bound, and deleted.
const AdminRoleID = "admin"

const adminRoleName = "admin"

// seedAdminRole offers the built-in admin role exactly once per installation.
// Deleting it afterwards is respected through State.AdminRoleSeeded. A registry
// that does not declare the management permissions is left untouched.
func seedAdminRole(
	ctx context.Context, registry *Registry, repo Repository, state State, now func() time.Time,
) error {
	if state.AdminRoleSeeded {
		return nil
	}
	rules := []RoleRule{
		{Permission: PermissionAssignmentsManage, Effect: Allow},
		{Permission: PermissionRolesManage, Effect: Allow},
	}
	for _, rule := range rules {
		if _, ok := registry.Lookup(rule.Permission); !ok {
			return nil
		}
	}
	err := repo.Mutate(ctx, func(state State) (State, []AuditEvent, error) {
		state.AdminRoleSeeded = true
		for _, role := range state.Roles {
			if role.ID == AdminRoleID || strings.EqualFold(role.Name, adminRoleName) {
				return state, []AuditEvent{{
					At: now().UnixMilli(), Actor: SystemActorName,
					Operation: AuditRoleCreated, RoleID: role.ID, Detail: "admin role already present",
				}}, nil
			}
		}
		now := now().UnixMilli()
		role := Role{
			ID: AdminRoleID, Name: adminRoleName,
			Description: "Built-in administrator role: manage permission assignments and roles.",
			Rules:       rules, CreatedBy: SystemActorName, CreatedAt: now, UpdatedAt: now,
		}
		state.Roles = append(state.Roles, role)
		return state, []AuditEvent{{
			At: now, Actor: SystemActorName, Operation: AuditRoleCreated,
			RoleID: AdminRoleID, Detail: describeRole(role),
		}}, nil
	})
	if err != nil {
		return fmt.Errorf("seed admin role: %w", err)
	}
	return nil
}
