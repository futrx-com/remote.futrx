package rbac

import (
	"context"
	"time"
)

// mutation owns one policy snapshot and its audit events while the repository
// holds its exclusive lock. Its dependencies are limited to policy decisions,
// identity checks, and the sources used to create mutation records.
type mutation struct {
	registry  *Registry
	identity  IdentityDirectory
	evaluator *Evaluator
	now       func() time.Time
	newID     func() (string, error)

	ctx    context.Context
	actor  Actor
	state  State
	events []AuditEvent
}

func (m *mutation) record(event AuditEvent) {
	event.At = m.now().UnixMilli()
	event.Actor = m.actor.Name()
	m.events = append(m.events, event)
}

func (m *mutation) evaluate(check Check) (Decision, error) {
	return m.evaluator.evaluate(m.ctx, m.state, m.actor, true, check)
}

// requireRegistered enforces that a permission target is an existing user.
func (m *mutation) requireRegistered(email string) error {
	registered, err := m.identity.IsRegistered(m.ctx, email)
	if err != nil {
		return err
	}
	if !registered {
		return ErrUserNotRegistered
	}
	return nil
}

// Delegation rules: "can give permissions" is itself a permission, but
// holding it is not an unrestricted route to permissions the delegator does
// not have. Every rule below is evaluated against the snapshot held under the
// store lock, so a concurrent revocation cannot slip past a stale check.

// authority records how an actor passed a management check.
type authority struct {
	// root is true for administrators and the trusted system actor, who may
	// mutate any registered permission.
	root bool
}

// requireManagement requires the platform-scoped management permission.
func (m *mutation) requireManagement(key Key) (authority, error) {
	decision, err := m.evaluate(Check{Permission: key, Scope: PlatformScope()})
	if err != nil {
		return authority{}, err
	}
	if !decision.Allowed {
		return authority{}, ErrDenied
	}
	return authority{root: decision.Reason == ReasonAdministrator || decision.Reason == ReasonSystem}, nil
}

// requireDelegable enforces that a non-root actor may hand out (or take back)
// key at scope: the permission must be marked Delegable and the actor must
// currently hold it at exactly that scope.
func (m *mutation) requireDelegable(by authority, key Key, scope Scope) error {
	if by.root {
		return nil
	}
	definition, ok := m.registry.Lookup(key)
	if !ok {
		return ErrUnknownPermission
	}
	if !definition.Delegable {
		return ErrDenied
	}
	decision, err := m.evaluate(Check{Permission: key, Scope: scope})
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return ErrDenied
	}
	return nil
}

// requireDelegableRules applies requireDelegable to every rule of a role at
// scope, whatever the rule's effect.
func (m *mutation) requireDelegableRules(by authority, rules []RoleRule, scope Scope) error {
	for _, rule := range rules {
		if err := m.requireDelegable(by, rule.Permission, scope); err != nil {
			return err
		}
	}
	return nil
}

// requireRoleContentHeld stops a non-root actor from writing a role that
// contains a management permission they do not themselves hold. Management
// permissions are platform scoped, so they are checked there.
func (m *mutation) requireRoleContentHeld(by authority, rules []RoleRule) error {
	if by.root {
		return nil
	}
	for _, rule := range rules {
		if rule.Permission != PermissionAssignmentsManage && rule.Permission != PermissionRolesManage {
			continue
		}
		decision, err := m.evaluate(Check{Permission: rule.Permission, Scope: PlatformScope()})
		if err != nil {
			return err
		}
		if !decision.Allowed {
			return ErrDenied
		}
	}
	return nil
}
