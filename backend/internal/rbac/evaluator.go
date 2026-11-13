package rbac

import (
	"context"
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/rbac/evaluator"
)

// Evaluator answers permission checks. It combines the code-owned registry
// with the persisted policy and consults narrow identity and membership ports
// rather than concrete auth or project services.
type Evaluator struct {
	registry *Registry
	repo     Repository
	identity IdentityDirectory
	members  ProjectMembership
}

// NewEvaluator builds an evaluator. members may be nil, in which case the
// project-member baseline admits only administrators.
func NewEvaluator(
	registry *Registry,
	repo Repository,
	identity IdentityDirectory,
	members ProjectMembership,
) *Evaluator {
	return &Evaluator{registry: registry, repo: repo, identity: identity, members: members}
}

// Evaluate decides check for the actor carried by ctx. The error is non-nil
// only when no decision could be made (missing actor, invalid check, or an
// unavailable dependency); callers must treat any error as a denial.
func (e *Evaluator) Evaluate(ctx context.Context, check Check) (Decision, error) {
	actor, ok := ActorFromContext(ctx)
	state, err := e.repo.Load(ctx)
	if err != nil {
		return Decision{Reason: ReasonDefaultDeny}, err
	}
	return e.evaluate(ctx, state, actor, ok, check)
}

// Can reports whether the actor carried by ctx may satisfy check.
func (e *Evaluator) Can(ctx context.Context, check Check) (bool, error) {
	decision, err := e.Evaluate(ctx, check)
	return decision.Allowed, err
}

// Require returns nil when the check passes and ErrDenied when it does not.
// The error deliberately omits which role or deny record decided the outcome.
func (e *Evaluator) Require(ctx context.Context, check Check) error {
	decision, err := e.Evaluate(ctx, check)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return ErrDenied
	}
	return nil
}

// evaluate resolves what the decision needs (definition, identity, membership)
// and delegates the decision itself to the pure evaluator package. Mutations
// call it with the snapshot they hold under the store lock so that
// authorization and write cannot be separated by a concurrent change.
func (e *Evaluator) evaluate(
	ctx context.Context,
	state State,
	actor Actor,
	actorPresent bool,
	check Check,
) (Decision, error) {
	if !actorPresent {
		return evaluator.Decide(Definition{}, state, evaluator.Facts{}, check)
	}
	definition, err := e.registry.resolve(check)
	if err != nil {
		return Decision{Reason: ReasonInvalidCheck}, err
	}
	facts := evaluator.Facts{ActorPresent: true, System: actor.IsSystem(), Email: actor.Email}
	if facts.System {
		return evaluator.Decide(definition, state, facts, check)
	}

	facts.Admin, err = e.identity.IsAdmin(ctx, actor.Email)
	if err != nil {
		return Decision{Reason: ReasonDefaultDeny}, err
	}
	if !facts.Admin {
		facts.Registered, err = e.identity.IsRegistered(ctx, actor.Email)
		if err != nil {
			return Decision{Reason: ReasonDefaultDeny}, err
		}
	}
	decision, err := evaluator.Decide(definition, state, facts, check)
	if !errors.Is(err, evaluator.ErrMembershipUnresolved) {
		return decision, err
	}
	// Only now does the outcome depend on membership; look it up once.
	facts.MembershipResolved = true
	if e.members != nil {
		facts.ProjectMember, facts.MembershipErr = e.members.HasAccess(ctx, check.Scope.ID, actor.Email)
	}
	return evaluator.Decide(definition, state, facts, check)
}
