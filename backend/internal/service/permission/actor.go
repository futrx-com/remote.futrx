package permission

import "context"

// Actor is the principal a permission decision is made for: either an
// authenticated human, identified by normalized email, or the trusted system.
//
// The system flag is unexported, so the only way to obtain a system actor is
// SystemActor or ContextWithSystemActor. Neither is ever fed from request
// input; reviewed internal entry points call them explicitly.
type Actor struct {
	Email  string
	system bool
}

// UserActor is the actor for an authenticated human.
func UserActor(email string) Actor { return Actor{Email: NormalizeEmail(email)} }

// SystemActor is the trusted internal actor for background work.
func SystemActor() Actor { return Actor{system: true} }

// IsSystem reports whether a is the trusted internal actor.
func (a Actor) IsSystem() bool { return a.system }

// Name is the identifier recorded in audit events.
func (a Actor) Name() string {
	if a.system {
		return SystemActorName
	}
	return a.Email
}

func (a Actor) present() bool { return a.system || a.Email != "" }

type actorContextKey struct{}

// ContextWithActor attaches the authenticated actor to ctx. Only code that
// has itself authenticated the caller, such as the auth middleware, may call
// it; never derive the actor from a request body, query, or method argument.
func ContextWithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actor)
}

// ContextWithSystemActor marks ctx as trusted internal work. Use it only at
// reviewed entry points; the architecture test lists the permitted callers.
func ContextWithSystemActor(ctx context.Context) context.Context {
	return ContextWithActor(ctx, SystemActor())
}

// ActorFromContext returns the actor attached to ctx. A missing actor is not
// the system: callers must fail closed with ErrActorRequired.
func ActorFromContext(ctx context.Context) (Actor, bool) {
	actor, ok := ctx.Value(actorContextKey{}).(Actor)
	if !ok || !actor.present() {
		return Actor{}, false
	}
	return actor, true
}
