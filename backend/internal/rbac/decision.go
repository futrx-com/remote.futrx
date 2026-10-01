package rbac

import (
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/rbac/evaluator"
	"github.com/futrx-com/remote.futrx.com/internal/rbac/models"
)

// Stable errors. Public errors never reveal which role or deny record caused a
// denial; the structured Decision carries that for logs and tests.
var (
	ErrDenied            = errors.New("permission denied")
	ErrActorRequired     = evaluator.ErrActorRequired
	ErrUnknownPermission = errors.New("unknown permission")
	ErrInvalidScope      = models.ErrInvalidScope
	ErrInvalidEffect     = errors.New("invalid permission effect")
	ErrInvalidDefinition = models.ErrInvalidDefinition
	ErrInvalidRole       = errors.New("invalid role")
	ErrRoleNotFound      = errors.New("role not found")
	ErrRoleInUse         = errors.New("role is bound to users")
	ErrUserNotRegistered = errors.New("user is not registered")
	ErrInvalidState      = models.ErrInvalidState
	ErrAuditFailed       = errors.New("permission audit failed")
)

// Reason and Decision live in the pure evaluator; they are re-exported by
// models.go.
