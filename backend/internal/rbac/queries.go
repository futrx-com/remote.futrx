package rbac

import "context"

// requireRead admits actors who hold either management permission. Policy is
// sensitive, so listing it is not open to every registered user.
func (s *Service) requireRead(ctx context.Context) (State, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return State{}, ErrActorRequired
	}
	state, err := s.repo.Load(ctx)
	if err != nil {
		return State{}, err
	}
	for _, key := range []Key{PermissionAssignmentsManage, PermissionRolesManage} {
		decision, err := s.evaluator.evaluate(ctx, state, actor, true, Check{Permission: key, Scope: PlatformScope()})
		if err != nil {
			return State{}, err
		}
		if decision.Allowed {
			return state, nil
		}
	}
	return State{}, ErrDenied
}

// Assignments lists every direct assignment.
func (s *Service) Assignments(ctx context.Context) ([]Assignment, error) {
	state, err := s.requireRead(ctx)
	return state.Assignments, err
}

// Roles lists every custom role.
func (s *Service) Roles(ctx context.Context) ([]Role, error) {
	state, err := s.requireRead(ctx)
	return state.Roles, err
}

// Bindings lists every role binding.
func (s *Service) Bindings(ctx context.Context) ([]RoleBinding, error) {
	state, err := s.requireRead(ctx)
	return state.Bindings, err
}

// Policy returns one authorized snapshot so the UI cannot combine records from
// different revisions. Definitions are code-owned and returned as copies.
func (s *Service) Policy(ctx context.Context) (State, []Definition, error) {
	state, err := s.requireRead(ctx)
	if err != nil {
		return State{}, nil, err
	}
	return state, s.registry.Definitions(), nil
}

// Effective returns only the current actor's decisions at an exact scope.
// It never exposes policy records or another user's decisions.
func (s *Service) Effective(ctx context.Context, scope Scope) (map[Key]bool, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return nil, ErrActorRequired
	}
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	state, err := s.repo.Load(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[Key]bool)
	for _, definition := range s.registry.Definitions() {
		if !definition.SupportsScope(scope.Kind) {
			continue
		}
		decision, err := s.evaluator.evaluate(ctx, state, actor, true, Check{Permission: definition.Key, Scope: scope})
		if err != nil {
			return nil, err
		}
		result[definition.Key] = decision.Allowed
	}
	return result, nil
}
