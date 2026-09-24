package permission

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
