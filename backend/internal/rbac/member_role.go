package rbac

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/rbac/models"
)

// MemberRoleID is the stable ID of the built-in member role. Like the admin
// role it is an ordinary stored role once seeded: it can be edited and
// deleted. It is never bound; the evaluator applies it to every registered
// non-administrator who holds no other platform role.
const MemberRoleID = models.MemberRoleID

const memberRoleName = "member"

// WithBuiltinMemberRole offers the built-in member role once at startup. It
// allows every permission whose baseline already admits members, plus extra:
// permissions such as provider-account use that have no baseline but that
// members need for the application to work.
func WithBuiltinMemberRole(extra ...Key) Option {
	return func(service *Service) {
		service.seedMember = true
		service.memberExtra = slices.Clone(extra)
	}
}

// seedMemberRole offers the built-in member role exactly once per
// installation. Deleting it afterwards is respected through
// State.MemberRoleSeeded, and members then fall back to the baselines.
func seedMemberRole(
	ctx context.Context, registry *Registry, repo Repository, extra []Key, now func() time.Time,
) error {
	var rules []RoleRule
	for _, definition := range registry.Definitions() {
		if definition.Baseline == BaselineAuthenticated || definition.Baseline == BaselineProjectMember ||
			slices.Contains(extra, definition.Key) {
			rules = append(rules, RoleRule{Permission: definition.Key, Effect: Allow})
		}
	}
	err := repo.Mutate(ctx, func(state State) (State, []AuditEvent, error) {
		if state.MemberRoleSeeded {
			return state, nil, nil
		}
		state.MemberRoleSeeded = true
		for _, role := range state.Roles {
			if role.ID == MemberRoleID || strings.EqualFold(role.Name, memberRoleName) {
				return state, []AuditEvent{{
					At: now().UnixMilli(), Actor: SystemActorName,
					Operation: AuditRoleCreated, RoleID: role.ID, Detail: "member role already present",
				}}, nil
			}
		}
		now := now().UnixMilli()
		role := Role{
			ID: MemberRoleID, Name: memberRoleName,
			Description: "Built-in default for every non-admin without another platform role. " +
				"Applies at every scope; project permissions still require project membership. " +
				"Set a permission to Deny to remove it from all members.",
			Rules: rules, CreatedBy: SystemActorName, CreatedAt: now, UpdatedAt: now,
		}
		state.Roles = append(state.Roles, role)
		return state, []AuditEvent{{
			At: now, Actor: SystemActorName, Operation: AuditRoleCreated,
			RoleID: MemberRoleID, Detail: describeRole(role),
		}}, nil
	})
	if err != nil {
		return fmt.Errorf("seed member role: %w", err)
	}
	return nil
}
