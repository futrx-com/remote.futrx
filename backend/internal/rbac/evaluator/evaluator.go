// Package evaluator is the pure allow/deny decision for one permission check.
// It performs no I/O and holds no context or registry: the caller resolves
// everything it needs into Facts before calling Decide.
package evaluator

import (
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/rbac/models"
)

// ErrActorRequired reports that no actor was present for the check.
var ErrActorRequired = errors.New("authenticated actor required")

// ErrMembershipUnresolved reports that the decision reached the project-member
// baseline before the caller resolved membership. The caller resolves it, sets
// Facts.MembershipResolved, and decides again. Membership is looked up lazily
// so a decision settled earlier never depends on that lookup.
var ErrMembershipUnresolved = errors.New("project membership not resolved")

// Reason explains a Decision for logs and tests.
type Reason string

const (
	ReasonSystem          Reason = "system"
	ReasonAdministrator   Reason = "administrator"
	ReasonExplicitDeny    Reason = "explicit-deny"
	ReasonExplicitAllow   Reason = "explicit-allow"
	ReasonBaseline        Reason = "baseline"
	ReasonDefaultDeny     Reason = "default-deny"
	ReasonUnknownActor    Reason = "unknown-actor"
	ReasonInvalidCheck    Reason = "invalid-check"
	ReasonActorNotPresent Reason = "no-actor"
)

// Decision is the structured outcome of one evaluation.
type Decision struct {
	Allowed bool
	Reason  Reason
}

// Facts is what the caller resolved before the call.
type Facts struct {
	// ActorPresent is false when the request carried no actor.
	ActorPresent bool
	// System marks the trusted internal actor.
	System bool
	// Email is the normalized actor identity; empty for the system actor.
	Email      string
	Admin      bool
	Registered bool
	// MembershipResolved is true once the caller has looked up membership.
	MembershipResolved bool
	// ProjectMember reports membership of the checked project. It is only
	// meaningful when MembershipResolved is set and MembershipErr is nil.
	ProjectMember bool
	// MembershipErr records a failed membership lookup. It fails the decision
	// only if the decision actually depends on membership.
	MembershipErr error
}

// Decide applies the policy in fixed order: fail closed on anything
// unresolved, administrator root, explicit deny, explicit allow (direct
// assignments and role rules through bindings), the code-owned baseline, and
// finally default deny. A non-nil error means no decision could be made and
// must be treated as a denial.
func Decide(definition models.Definition, policy models.State, facts Facts, check models.Check) (Decision, error) {
	if !facts.ActorPresent {
		return Decision{Reason: ReasonActorNotPresent}, ErrActorRequired
	}
	if facts.System {
		return Decision{Allowed: true, Reason: ReasonSystem}, nil
	}
	if facts.Admin {
		return Decision{Allowed: true, Reason: ReasonAdministrator}, nil
	}
	if !facts.Registered {
		return Decision{Reason: ReasonUnknownActor}, nil
	}

	deny, allow := matchingEffects(policy, facts.Email, check)
	if deny {
		return Decision{Reason: ReasonExplicitDeny}, nil
	}
	if allow {
		return Decision{Allowed: true, Reason: ReasonExplicitAllow}, nil
	}

	baseline, err := baseline(definition.Baseline, facts, check)
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
func matchingEffects(state models.State, email string, check models.Check) (deny, allow bool) {
	record := func(effect models.Effect) {
		if effect == models.Deny {
			deny = true
		} else if effect == models.Allow {
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
		role, ok := state.Role(b.RoleID)
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
func baseline(policy models.BaselinePolicy, facts Facts, check models.Check) (bool, error) {
	switch policy {
	case models.BaselineAuthenticated:
		return true, nil
	case models.BaselineProjectMember:
		if check.Scope.Kind != models.ScopeProject {
			return false, nil
		}
		if !facts.MembershipResolved {
			return false, ErrMembershipUnresolved
		}
		if facts.MembershipErr != nil {
			return false, facts.MembershipErr
		}
		return facts.ProjectMember, nil
	case models.BaselineAdmin, models.BaselineNone:
		return false, nil
	}
	return false, errors.New("unreachable baseline policy: " + string(policy))
}
