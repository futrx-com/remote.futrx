package permission

import (
	"context"
	"fmt"
	"strings"
)

const (
	maxRoleNameLength        = 80
	maxRoleDescriptionLength = 500
)

// RoleInput is the editable content of a custom role.
type RoleInput struct {
	Name        string
	Description string
	Rules       []RoleRule
}

// DeleteRoleOptions controls deletion of a role that still has bindings.
type DeleteRoleOptions struct {
	// Unbind atomically removes every binding along with the role. Without
	// it, deleting a bound role fails with ErrRoleInUse.
	Unbind bool
}

// validateRole normalizes and validates role content against the registry.
func (s *Service) validateRole(state State, selfID string, in RoleInput) (RoleInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" || len(in.Name) > maxRoleNameLength {
		return RoleInput{}, fmt.Errorf("%w: name must be 1-%d characters", ErrInvalidRole, maxRoleNameLength)
	}
	if len(in.Description) > maxRoleDescriptionLength {
		return RoleInput{}, fmt.Errorf("%w: description exceeds %d characters", ErrInvalidRole, maxRoleDescriptionLength)
	}
	for _, other := range state.Roles {
		if other.ID != selfID && strings.EqualFold(other.Name, in.Name) {
			return RoleInput{}, fmt.Errorf("%w: name %q is already in use", ErrInvalidRole, in.Name)
		}
	}
	if len(in.Rules) == 0 {
		return RoleInput{}, fmt.Errorf("%w: at least one rule is required", ErrInvalidRole)
	}
	seen := make(map[Key]struct{}, len(in.Rules))
	for _, rule := range in.Rules {
		if _, ok := s.registry.Lookup(rule.Permission); !ok {
			return RoleInput{}, fmt.Errorf("%w: %q", ErrUnknownPermission, rule.Permission)
		}
		if !rule.Effect.Valid() {
			return RoleInput{}, fmt.Errorf("%w: %q", ErrInvalidEffect, rule.Effect)
		}
		if _, dup := seen[rule.Permission]; dup {
			return RoleInput{}, fmt.Errorf("%w: %s appears more than once", ErrInvalidRole, rule.Permission)
		}
		seen[rule.Permission] = struct{}{}
	}
	in.Rules = append([]RoleRule(nil), in.Rules...)
	return in, nil
}

// CreateRole adds a custom role.
func (s *Service) CreateRole(ctx context.Context, in RoleInput) (Role, error) {
	var result Role
	err := s.mutate(ctx, func(m *mutation) error {
		by, err := m.requireManagement(PermissionRolesManage)
		if err != nil {
			return err
		}
		valid, err := s.validateRole(m.state, "", in)
		if err != nil {
			return err
		}
		if err := m.requireRoleContentHeld(by, valid.Rules); err != nil {
			return err
		}
		id, err := m.service.newID()
		if err != nil {
			return err
		}
		now := m.service.now().UnixMilli()
		result = Role{
			ID: id, Name: valid.Name, Description: valid.Description, Rules: valid.Rules,
			CreatedBy: m.actor.Name(), CreatedAt: now, UpdatedAt: now,
		}
		m.state.Roles = append(m.state.Roles, result)
		m.record(AuditEvent{Operation: AuditRoleCreated, RoleID: id, Detail: describeRole(result)})
		return nil
	})
	if err != nil {
		return Role{}, err
	}
	return result, nil
}

// UpdateRole replaces a role's content. Bindings follow the role, so a change
// reaches every bound user without rewriting them. Because it can therefore
// widen what others hold, a non-administrator must be able to delegate every
// old and new rule at each existing binding's scope.
func (s *Service) UpdateRole(ctx context.Context, id string, in RoleInput) (Role, error) {
	var result Role
	err := s.mutate(ctx, func(m *mutation) error {
		by, err := m.requireManagement(PermissionRolesManage)
		if err != nil {
			return err
		}
		index := -1
		for i, role := range m.state.Roles {
			if role.ID == id {
				index = i
			}
		}
		if index < 0 {
			return ErrRoleNotFound
		}
		existing := m.state.Roles[index]
		valid, err := s.validateRole(m.state, id, in)
		if err != nil {
			return err
		}
		if err := m.requireRoleContentHeld(by, valid.Rules); err != nil {
			return err
		}
		for _, binding := range m.state.Bindings {
			if binding.RoleID != id {
				continue
			}
			if err := s.requireRulesSupportScope(valid.Rules, binding.Scope); err != nil {
				return err
			}
			if err := m.requireDelegableRules(by, existing.Rules, binding.Scope); err != nil {
				return err
			}
			if err := m.requireDelegableRules(by, valid.Rules, binding.Scope); err != nil {
				return err
			}
		}
		updated := existing
		updated.Name, updated.Description, updated.Rules = valid.Name, valid.Description, valid.Rules
		updated.UpdatedAt = m.service.now().UnixMilli()
		m.state.Roles[index] = updated
		m.record(AuditEvent{Operation: AuditRoleUpdated, RoleID: id, Detail: describeRole(updated)})
		result = updated
		return nil
	})
	if err != nil {
		return Role{}, err
	}
	return result, nil
}

// DeleteRole removes a custom role. Deleting a missing role is a no-op.
func (s *Service) DeleteRole(ctx context.Context, id string, options DeleteRoleOptions) error {
	return s.mutate(ctx, func(m *mutation) error {
		by, err := m.requireManagement(PermissionRolesManage)
		if err != nil {
			return err
		}
		role, ok := m.state.role(id)
		if !ok {
			return nil
		}

		var bound []RoleBinding
		kept := m.state.Bindings[:0:0]
		for _, binding := range m.state.Bindings {
			if binding.RoleID == id {
				bound = append(bound, binding)
			} else {
				kept = append(kept, binding)
			}
		}
		if len(bound) > 0 {
			if !options.Unbind {
				return ErrRoleInUse
			}
			// Unbinding is assignment management, and it takes back what the
			// role granted, so it follows the binding rules.
			if _, err := m.requireManagement(PermissionAssignmentsManage); err != nil {
				return err
			}
			for _, binding := range bound {
				if err := m.requireDelegableRules(by, role.Rules, binding.Scope); err != nil {
					return err
				}
				m.record(AuditEvent{
					Operation: AuditRoleUnbound, TargetUser: binding.UserEmail, RoleID: id,
					Scope: binding.Scope, Detail: role.Name,
				})
			}
			m.state.Bindings = kept
		}

		roles := m.state.Roles[:0:0]
		for _, other := range m.state.Roles {
			if other.ID != id {
				roles = append(roles, other)
			}
		}
		m.state.Roles = roles
		m.record(AuditEvent{Operation: AuditRoleDeleted, RoleID: id, Detail: role.Name})
		return nil
	})
}

func describeRole(role Role) string {
	parts := make([]string, 0, len(role.Rules))
	for _, rule := range role.Rules {
		parts = append(parts, string(rule.Effect)+" "+string(rule.Permission))
	}
	return role.Name + ": " + strings.Join(parts, ", ")
}
