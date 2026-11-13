package rbac

import "context"

// SystemOnlyAuthorizer is the fail-closed default for services without an
// injected policy authorizer. Only explicitly trusted internal work is allowed;
// an absent actor is distinct from an authenticated user who is denied.
type SystemOnlyAuthorizer struct{}

func (SystemOnlyAuthorizer) Require(ctx context.Context, _ Check) error {
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return ErrActorRequired
	}
	if !actor.IsSystem() {
		return ErrDenied
	}
	return nil
}
