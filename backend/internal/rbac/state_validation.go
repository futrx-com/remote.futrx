package rbac

import "fmt"

// ValidateAgainst checks the state against the code-owned catalog. Stored
// policy that names an unregistered key, or applies a permission at a scope
// kind it does not support, is an error rather than something to discard:
// silently dropping a deny would weaken access.
func ValidateAgainst(s State, registry *Registry) error {
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
		role, _ := s.Role(b.RoleID)
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
