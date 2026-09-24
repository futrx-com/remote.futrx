package permission

import (
	"fmt"
	"slices"
)

// Registry is the immutable, code-owned catalog of permission definitions. It
// stores no assignments and decides no access; the Evaluator does that.
type Registry struct {
	definitions map[Key]Definition
	keys        []Key
}

// NewRegistry validates the definition groups and freezes them into a
// registry. A group is what one owning service exports from its
// PermissionDefinitions function.
func NewRegistry(groups ...[]Definition) (*Registry, error) {
	registry := &Registry{definitions: make(map[Key]Definition)}
	for _, group := range groups {
		for _, definition := range group {
			if definition.Baseline == "" {
				definition.Baseline = BaselineNone
			}
			if err := definition.validate(); err != nil {
				return nil, err
			}
			if _, dup := registry.definitions[definition.Key]; dup {
				return nil, fmt.Errorf("%w: duplicate key %q", ErrInvalidDefinition, definition.Key)
			}
			definition.Scopes = slices.Clone(definition.Scopes)
			registry.definitions[definition.Key] = definition
			registry.keys = append(registry.keys, definition.Key)
		}
	}
	if len(registry.keys) == 0 {
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, errNilDefinitions)
	}
	slices.Sort(registry.keys)
	return registry, nil
}

// MustRegistry is NewRegistry for the composition root, where an invalid
// catalog is a programming error that must stop startup.
func MustRegistry(groups ...[]Definition) *Registry {
	registry, err := NewRegistry(groups...)
	if err != nil {
		panic(err)
	}
	return registry
}

// Lookup returns the definition registered for key.
func (r *Registry) Lookup(key Key) (Definition, bool) {
	definition, ok := r.definitions[key]
	if ok {
		definition.Scopes = slices.Clone(definition.Scopes)
	}
	return definition, ok
}

// Definitions returns every definition sorted by key.
func (r *Registry) Definitions() []Definition {
	out := make([]Definition, 0, len(r.keys))
	for _, key := range r.keys {
		definition, _ := r.Lookup(key)
		out = append(out, definition)
	}
	return out
}

// resolve validates a check against the catalog: the key must be registered
// and the scope well formed and supported by that definition.
func (r *Registry) resolve(check Check) (Definition, error) {
	definition, ok := r.Lookup(check.Permission)
	if !ok {
		return Definition{}, fmt.Errorf("%w: %q", ErrUnknownPermission, check.Permission)
	}
	if err := check.Scope.Validate(); err != nil {
		return Definition{}, err
	}
	if !definition.SupportsScope(check.Scope.Kind) {
		return Definition{}, fmt.Errorf("%w: %s does not apply to %s scope",
			ErrInvalidScope, check.Permission, check.Scope.Kind)
	}
	return definition, nil
}

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
