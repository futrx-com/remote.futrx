package permission

import (
	"context"
	"errors"
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

// evaluate is the pure decision procedure over one policy snapshot. Mutations
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
		return Decision{Reason: ReasonActorNotPresent}, ErrActorRequired
	}
	definition, err := e.registry.resolve(check)
	if err != nil {
		return Decision{Reason: ReasonInvalidCheck}, err
	}
	if actor.IsSystem() {
		return Decision{Allowed: true, Reason: ReasonSystem}, nil
	}

	admin, err := e.identity.IsAdmin(ctx, actor.Email)
	if err != nil {
		return Decision{Reason: ReasonDefaultDeny}, err
	}
	if admin {
		return Decision{Allowed: true, Reason: ReasonAdministrator}, nil
	}
	registered, err := e.identity.IsRegistered(ctx, actor.Email)
	if err != nil {
		return Decision{Reason: ReasonDefaultDeny}, err
	}
	if !registered {
		return Decision{Reason: ReasonUnknownActor}, nil
	}

	deny, allow := matchingEffects(state, actor.Email, check)
	if deny {
		return Decision{Reason: ReasonExplicitDeny}, nil
	}
	if allow {
		return Decision{Allowed: true, Reason: ReasonExplicitAllow}, nil
	}

	baseline, err := e.baseline(ctx, definition.Baseline, actor, check)
	if err != nil {
		return Decision{Reason: ReasonDefaultDeny}, err
	}
	if baseline {
		return Decision{Allowed: true, Reason: ReasonBaseline}, nil
	}
	return Decision{Reason: ReasonDefaultDeny}, nil
}

// matchingEffects collects the direct assignments and bound-role rules that
// name check's permission at exactly check's scope. Scope matching is exact:
// a platform rule never matches a project check, and project A never matches
// project B.
func matchingEffects(state State, email string, check Check) (deny, allow bool) {
	record := func(effect Effect) {
		if effect == Deny {
			deny = true
		} else if effect == Allow {
			allow = true
		}
	}
	for _, a := range state.Assignments {
		if a.UserEmail == email && a.Permission == check.Permission && a.Scope == check.Scope {
			record(a.Effect)
		}
	}
	for _, b := range state.Bindings {
		if b.UserEmail != email || b.Scope != check.Scope {
			continue
		}
		role, ok := state.role(b.RoleID)
		if !ok {
			continue
		}
		for _, rule := range role.Rules {
			if rule.Permission == check.Permission {
				record(rule.Effect)
			}
		}
	}
	return deny, allow
}

// baseline evaluates the code-owned compatibility policy for a non-admin
// registered actor. Administrators were already admitted by the root policy.
func (e *Evaluator) baseline(
	ctx context.Context,
	policy BaselinePolicy,
	actor Actor,
	check Check,
) (bool, error) {
	switch policy {
	case BaselineAuthenticated:
		return true, nil
	case BaselineProjectMember:
		if check.Scope.Kind != ScopeProject || e.members == nil {
			return false, nil
		}
		return e.members.HasAccess(ctx, check.Scope.ID, actor.Email)
	case BaselineAdmin, BaselineNone:
		return false, nil
	}
	return false, errors.New("unreachable baseline policy: " + string(policy))
}
