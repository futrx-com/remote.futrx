package rbac

import (
	"errors"
	"fmt"
	"slices"
)

var errNilDefinitions = errors.New("no definitions")

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
			if err := definition.Validate(); err != nil {
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
	if err := requireScopeSupport(definition, check.Scope.Kind); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

// requireRulesSupportScope keeps a project role from becoming platform-wide:
// every rule's permission must be declared for the binding's scope kind.
func (r *Registry) requireRulesSupportScope(rules []RoleRule, kind ScopeKind) error {
	for _, rule := range rules {
		definition, ok := r.Lookup(rule.Permission)
		if !ok {
			return fmt.Errorf("%w: %q", ErrUnknownPermission, rule.Permission)
		}
		if err := requireScopeSupport(definition, kind); err != nil {
			return err
		}
	}
	return nil
}

// requireScopeSupport is shared by individual checks and role binding rules.
func requireScopeSupport(definition Definition, kind ScopeKind) error {
	if !definition.SupportsScope(kind) {
		return fmt.Errorf("%w: %s does not apply to %s scope", ErrInvalidScope, definition.Key, kind)
	}
	return nil
}
