package permission

import (
	"context"
	"fmt"
)

// AssignmentInput describes one direct grant or deny.
type AssignmentInput struct {
	UserEmail  string
	Permission Key
	Effect     Effect
	Scope      Scope
}

// AssignmentTarget identifies a direct assignment to remove.
type AssignmentTarget struct {
	UserEmail  string
	Permission Key
	Scope      Scope
}

// BindingInput describes a role binding.
type BindingInput struct {
	RoleID    string
	UserEmail string
	Scope     Scope
}

// SetAssignment creates or replaces the direct assignment for the input's
// (user, permission, scope). Re-applying the same effect is a no-op.
func (s *Service) SetAssignment(ctx context.Context, in AssignmentInput) (Assignment, error) {
	email := NormalizeEmail(in.UserEmail)
	if email == "" {
		return Assignment{}, fmt.Errorf("%w: user is required", ErrUserNotRegistered)
	}
	if !in.Effect.Valid() {
		return Assignment{}, fmt.Errorf("%w: %q", ErrInvalidEffect, in.Effect)
	}
	if _, err := s.registry.resolve(Check{Permission: in.Permission, Scope: in.Scope}); err != nil {
		return Assignment{}, err
	}

	var result Assignment
	err := s.mutate(ctx, func(m *mutation) error {
		by, err := m.requireManagement(PermissionAssignmentsManage)
		if err != nil {
			return err
		}
		if err := m.requireDelegable(by, in.Permission, in.Scope); err != nil {
			return err
		}
		if err := m.requireRegistered(email); err != nil {
			return err
		}

		for i, existing := range m.state.Assignments {
			if existing.UserEmail != email || existing.Permission != in.Permission || existing.Scope != in.Scope {
				continue
			}
			result = existing
			if existing.Effect == in.Effect {
				return nil
			}
			m.state.Assignments[i].Effect = in.Effect
			result = m.state.Assignments[i]
			m.record(AuditEvent{
				Operation: AuditAssignmentSet, TargetUser: email, Permission: in.Permission,
				Scope: in.Scope, OldEffect: existing.Effect, NewEffect: in.Effect,
			})
			return nil
		}

		id, err := m.service.newID()
		if err != nil {
			return err
		}
		result = Assignment{
			ID: id, UserEmail: email, Permission: in.Permission, Effect: in.Effect, Scope: in.Scope,
			CreatedBy: m.actor.Name(), CreatedAt: m.service.now().UnixMilli(),
		}
		m.state.Assignments = append(m.state.Assignments, result)
		m.record(AuditEvent{
			Operation: AuditAssignmentSet, TargetUser: email, Permission: in.Permission,
			Scope: in.Scope, NewEffect: in.Effect,
		})
		return nil
	})
	if err != nil {
		return Assignment{}, err
	}
	return result, nil
}

// RemoveAssignment deletes a direct assignment. Removing one that does not
// exist is a no-op, but the caller is still authorized first.
func (s *Service) RemoveAssignment(ctx context.Context, target AssignmentTarget) error {
	email := NormalizeEmail(target.UserEmail)
	if _, err := s.registry.resolve(Check{Permission: target.Permission, Scope: target.Scope}); err != nil {
		return err
	}
	return s.mutate(ctx, func(m *mutation) error {
		by, err := m.requireManagement(PermissionAssignmentsManage)
		if err != nil {
			return err
		}
		if err := m.requireDelegable(by, target.Permission, target.Scope); err != nil {
			return err
		}
		for i, existing := range m.state.Assignments {
			if existing.UserEmail != email || existing.Permission != target.Permission || existing.Scope != target.Scope {
				continue
			}
			m.state.Assignments = append(m.state.Assignments[:i], m.state.Assignments[i+1:]...)
			m.record(AuditEvent{
				Operation: AuditAssignmentRemoved, TargetUser: email, Permission: target.Permission,
				Scope: target.Scope, OldEffect: existing.Effect,
			})
			return nil
		}
		return nil
	})
}

// BindRole attaches a role to a user at a concrete scope. Binding the same
// (user, role, scope) twice is a no-op.
func (s *Service) BindRole(ctx context.Context, in BindingInput) (RoleBinding, error) {
	email := NormalizeEmail(in.UserEmail)
	if email == "" {
		return RoleBinding{}, fmt.Errorf("%w: user is required", ErrUserNotRegistered)
	}
	if err := in.Scope.Validate(); err != nil {
		return RoleBinding{}, err
	}

	var result RoleBinding
	err := s.mutate(ctx, func(m *mutation) error {
		by, err := m.requireManagement(PermissionAssignmentsManage)
		if err != nil {
			return err
		}
		role, ok := m.state.role(in.RoleID)
		if !ok {
			return ErrRoleNotFound
		}
		if err := m.service.requireRulesSupportScope(role.Rules, in.Scope); err != nil {
			return err
		}
		if err := m.requireDelegableRules(by, role.Rules, in.Scope); err != nil {
			return err
		}
		if err := m.requireRegistered(email); err != nil {
			return err
		}

		for _, existing := range m.state.Bindings {
			if existing.RoleID == in.RoleID && existing.UserEmail == email && existing.Scope == in.Scope {
				result = existing
				return nil
			}
		}
		id, err := m.service.newID()
		if err != nil {
			return err
		}
		result = RoleBinding{
			ID: id, RoleID: in.RoleID, UserEmail: email, Scope: in.Scope,
			CreatedBy: m.actor.Name(), CreatedAt: m.service.now().UnixMilli(),
		}
		m.state.Bindings = append(m.state.Bindings, result)
		m.record(AuditEvent{
			Operation: AuditRoleBound, TargetUser: email, RoleID: in.RoleID, Scope: in.Scope, Detail: role.Name,
		})
		return nil
	})
	if err != nil {
		return RoleBinding{}, err
	}
	return result, nil
}

// UnbindRole removes a role binding. Removing a missing binding is a no-op.
func (s *Service) UnbindRole(ctx context.Context, in BindingInput) error {
	email := NormalizeEmail(in.UserEmail)
	if err := in.Scope.Validate(); err != nil {
		return err
	}
	return s.mutate(ctx, func(m *mutation) error {
		by, err := m.requireManagement(PermissionAssignmentsManage)
		if err != nil {
			return err
		}
		role, ok := m.state.role(in.RoleID)
		if !ok {
			return nil
		}
		if err := m.requireDelegableRules(by, role.Rules, in.Scope); err != nil {
			return err
		}
		for i, existing := range m.state.Bindings {
			if existing.RoleID != in.RoleID || existing.UserEmail != email || existing.Scope != in.Scope {
				continue
			}
			m.state.Bindings = append(m.state.Bindings[:i], m.state.Bindings[i+1:]...)
			m.record(AuditEvent{
				Operation: AuditRoleUnbound, TargetUser: email, RoleID: in.RoleID, Scope: in.Scope, Detail: role.Name,
			})
			return nil
		}
		return nil
	})
}

// RemoveUserPolicy deletes every direct assignment and role binding for a
// removed user, keeping role definitions. It is the trusted cleanup hook for
// user removal, not an operator entry point, so it performs no delegation
// check; the audit events name the system as the actor. It is idempotent.
func (s *Service) RemoveUserPolicy(ctx context.Context, email string) error {
	email = NormalizeEmail(email)
	if email == "" {
		return nil
	}
	return s.mutate(ContextWithSystemActor(ctx), func(m *mutation) error {
		keptAssignments := m.state.Assignments[:0:0]
		for _, existing := range m.state.Assignments {
			if existing.UserEmail != email {
				keptAssignments = append(keptAssignments, existing)
				continue
			}
			m.record(AuditEvent{
				Operation: AuditAssignmentRemoved, TargetUser: email, Permission: existing.Permission,
				Scope: existing.Scope, OldEffect: existing.Effect, Detail: "user removed",
			})
		}
		keptBindings := m.state.Bindings[:0:0]
		for _, existing := range m.state.Bindings {
			if existing.UserEmail != email {
				keptBindings = append(keptBindings, existing)
				continue
			}
			m.record(AuditEvent{
				Operation: AuditRoleUnbound, TargetUser: email, RoleID: existing.RoleID,
				Scope: existing.Scope, Detail: "user removed",
			})
		}
		m.state.Assignments = keptAssignments
		m.state.Bindings = keptBindings
		return nil
	})
}

// requireRulesSupportScope keeps a project role from becoming platform-wide:
// every rule's permission must be declared for the binding's scope kind.
func (s *Service) requireRulesSupportScope(rules []RoleRule, scope Scope) error {
	for _, rule := range rules {
		definition, ok := s.registry.Lookup(rule.Permission)
		if !ok {
			return fmt.Errorf("%w: %q", ErrUnknownPermission, rule.Permission)
		}
		if !definition.SupportsScope(scope.Kind) {
			return fmt.Errorf("%w: %s does not apply to %s scope", ErrInvalidScope, rule.Permission, scope.Kind)
		}
	}
	return nil
}
