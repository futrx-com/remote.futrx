package permission

import (
	"fmt"
	"slices"
	"strings"
)

// Assignment is one direct allow or deny of a permission to a user at a scope.
type Assignment struct {
	ID         string
	UserEmail  string
	Permission Key
	Effect     Effect
	Scope      Scope
	CreatedBy  string
	CreatedAt  int64
}

// RoleRule is one permission a role contributes when bound. The binding, not
// the rule, supplies the concrete scope.
type RoleRule struct {
	Permission Key
	Effect     Effect
}

// Role is a reusable bundle of rules. It is independent of the user
// directory's fixed admin/member roles.
type Role struct {
	ID          string
	Name        string
	Description string
	Rules       []RoleRule
	CreatedBy   string
	CreatedAt   int64
	UpdatedAt   int64
}

// RoleBinding attaches a role to a user at a concrete scope.
type RoleBinding struct {
	ID        string
	RoleID    string
	UserEmail string
	Scope     Scope
	CreatedBy string
	CreatedAt int64
}

// State is the complete persisted policy. The zero value is an empty policy,
// which is exactly the state of an installation that has never assigned
// anything.
type State struct {
	Assignments []Assignment
	Roles       []Role
	Bindings    []RoleBinding
}

// NormalizeEmail is the one canonical form of a user reference.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Clone returns a deep copy, so callers can edit a snapshot freely.
func (s State) Clone() State {
	out := State{
		Assignments: slices.Clone(s.Assignments),
		Roles:       make([]Role, len(s.Roles)),
		Bindings:    slices.Clone(s.Bindings),
	}
	for i, role := range s.Roles {
		role.Rules = slices.Clone(role.Rules)
		out.Roles[i] = role
	}
	return out
}

func (s State) role(id string) (Role, bool) {
	for _, role := range s.Roles {
		if role.ID == id {
			return role, true
		}
	}
	return Role{}, false
}

// ValidateStructure checks everything that does not need the registry:
// unique record IDs, well-formed scopes, effects, and emails, no duplicated
// natural keys, and no binding that references a missing role.
func (s State) ValidateStructure() error {
	ids := make(map[string]string)
	claim := func(kind, id string) error {
		if id == "" {
			return fmt.Errorf("%w: %s has an empty id", ErrInvalidState, kind)
		}
		if previous, dup := ids[id]; dup {
			return fmt.Errorf("%w: id %q is used by both a %s and a %s", ErrInvalidState, id, previous, kind)
		}
		ids[id] = kind
		return nil
	}

	assigned := make(map[string]struct{})
	for _, a := range s.Assignments {
		if err := claim("assignment", a.ID); err != nil {
			return err
		}
		if a.UserEmail == "" || a.UserEmail != NormalizeEmail(a.UserEmail) {
			return fmt.Errorf("%w: assignment %s has a malformed user", ErrInvalidState, a.ID)
		}
		if !a.Effect.Valid() {
			return fmt.Errorf("%w: assignment %s has effect %q", ErrInvalidState, a.ID, a.Effect)
		}
		if err := a.Scope.Validate(); err != nil {
			return fmt.Errorf("%w: assignment %s: %w", ErrInvalidState, a.ID, err)
		}
		natural := strings.Join([]string{a.UserEmail, string(a.Permission), a.Scope.String()}, "\x00")
		if _, dup := assigned[natural]; dup {
			return fmt.Errorf("%w: assignment %s duplicates another for %s", ErrInvalidState, a.ID, a.UserEmail)
		}
		assigned[natural] = struct{}{}
	}

	for _, role := range s.Roles {
		if err := claim("role", role.ID); err != nil {
			return err
		}
		if strings.TrimSpace(role.Name) == "" {
			return fmt.Errorf("%w: role %s has no name", ErrInvalidState, role.ID)
		}
		for _, rule := range role.Rules {
			if !rule.Effect.Valid() {
				return fmt.Errorf("%w: role %s has effect %q", ErrInvalidState, role.ID, rule.Effect)
			}
		}
	}

	bound := make(map[string]struct{})
	for _, b := range s.Bindings {
		if err := claim("binding", b.ID); err != nil {
			return err
		}
		if b.UserEmail == "" || b.UserEmail != NormalizeEmail(b.UserEmail) {
			return fmt.Errorf("%w: binding %s has a malformed user", ErrInvalidState, b.ID)
		}
		if err := b.Scope.Validate(); err != nil {
			return fmt.Errorf("%w: binding %s: %w", ErrInvalidState, b.ID, err)
		}
		if _, ok := s.role(b.RoleID); !ok {
			return fmt.Errorf("%w: binding %s references unknown role %q; restore the role or remove the binding from permissions.json",
				ErrInvalidState, b.ID, b.RoleID)
		}
		natural := strings.Join([]string{b.UserEmail, b.RoleID, b.Scope.String()}, "\x00")
		if _, dup := bound[natural]; dup {
			return fmt.Errorf("%w: binding %s duplicates another for %s", ErrInvalidState, b.ID, b.UserEmail)
		}
		bound[natural] = struct{}{}
	}
	return nil
}

// ValidateAgainst checks the state against the code-owned catalog. Stored
// policy that names an unregistered key, or applies a permission at a scope
// kind it does not support, is an error rather than something to discard:
// silently dropping a deny would weaken access.
func (s State) ValidateAgainst(registry *Registry) error {
	for _, a := range s.Assignments {
		definition, ok := registry.Lookup(a.Permission)
		if !ok {
			return fmt.Errorf("%w: assignment %s names unknown permission %q; register it or remove the assignment from permissions.json",
				ErrInvalidState, a.ID, a.Permission)
		}
		if !definition.SupportsScope(a.Scope.Kind) {
			return fmt.Errorf("%w: assignment %s applies %s at unsupported %s scope",
				ErrInvalidState, a.ID, a.Permission, a.Scope.Kind)
		}
	}
	for _, role := range s.Roles {
		for _, rule := range role.Rules {
			if _, ok := registry.Lookup(rule.Permission); !ok {
				return fmt.Errorf("%w: role %s names unknown permission %q; register it or remove the rule from permissions.json",
					ErrInvalidState, role.ID, rule.Permission)
			}
		}
	}
	for _, b := range s.Bindings {
		role, _ := s.role(b.RoleID)
		for _, rule := range role.Rules {
			definition, _ := registry.Lookup(rule.Permission)
			if !definition.SupportsScope(b.Scope.Kind) {
				return fmt.Errorf("%w: binding %s applies role %s rule %s at unsupported %s scope",
					ErrInvalidState, b.ID, role.ID, rule.Permission, b.Scope.Kind)
			}
		}
	}
	return nil
}

// AuditOperation names the kind of policy mutation an AuditEvent records.
type AuditOperation string

const (
	AuditAssignmentSet     AuditOperation = "assignment.set"
	AuditAssignmentRemoved AuditOperation = "assignment.removed"
	AuditRoleCreated       AuditOperation = "role.created"
	AuditRoleUpdated       AuditOperation = "role.updated"
	AuditRoleDeleted       AuditOperation = "role.deleted"
	AuditRoleBound         AuditOperation = "binding.created"
	AuditRoleUnbound       AuditOperation = "binding.removed"
)

// SystemActorName is recorded as the actor of trusted internal mutations.
const SystemActorName = "system"

// AuditEvent is one durable record of a successful policy mutation. It never
// carries session material or unrelated user data.
type AuditEvent struct {
	At         int64
	Actor      string
	Operation  AuditOperation
	TargetUser string
	Permission Key
	RoleID     string
	Scope      Scope
	OldEffect  Effect
	NewEffect  Effect
	// Detail carries what the other fields cannot, such as a role's name and
	// rules.
	Detail string
}
