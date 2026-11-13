package rbac

// Management permissions gate the permission service itself.
const (
	PermissionAssignmentsManage Key = "permissions.assignments.manage"
	PermissionRolesManage       Key = "permissions.roles.manage"
)

// ManagementDefinitions declares the permissions that authorize changing
// policy. They are not delegable: only administrators hand them out, so a
// manager can never widen who else may manage policy.
func ManagementDefinitions() []Definition {
	return []Definition{
		{
			Key:         PermissionAssignmentsManage,
			Description: "Create, change, and remove permission assignments and role bindings.",
			Scopes:      []ScopeKind{ScopePlatform},
			Baseline:    BaselineNone,
		},
		{
			Key:         PermissionRolesManage,
			Description: "Create, change, and delete custom roles.",
			Scopes:      []ScopeKind{ScopePlatform},
			Baseline:    BaselineNone,
		},
	}
}
