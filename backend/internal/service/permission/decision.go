package permission

import "errors"

// Stable errors. Public errors never reveal which role or deny record caused a
// denial; the structured Decision carries that for logs and tests.
var (
	ErrDenied            = errors.New("permission denied")
	ErrActorRequired     = errors.New("authenticated actor required")
	ErrUnknownPermission = errors.New("unknown permission")
	ErrInvalidScope      = errors.New("invalid permission scope")
	ErrInvalidEffect     = errors.New("invalid permission effect")
	ErrInvalidDefinition = errors.New("invalid permission definition")
	ErrInvalidRole       = errors.New("invalid role")
	ErrRoleNotFound      = errors.New("role not found")
	ErrRoleInUse         = errors.New("role is bound to users")
	ErrUserNotRegistered = errors.New("user is not registered")
	ErrInvalidState      = errors.New("invalid permission state")
	ErrAuditFailed       = errors.New("permission audit failed")
)

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
