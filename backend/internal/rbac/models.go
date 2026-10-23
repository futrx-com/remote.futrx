package rbac

import (
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/rbac/evaluator"
	"github.com/futrx-com/remote.futrx.com/internal/rbac/models"
)

// Model aliases keep the RBAC API stable while the records live in models.
type Key = models.Key
type Effect = models.Effect

const (
	Allow = models.Allow
	Deny  = models.Deny
)

type ScopeKind = models.ScopeKind

const (
	ScopePlatform = models.ScopePlatform
	ScopeProject  = models.ScopeProject
)

type Scope = models.Scope

func PlatformScope() Scope         { return models.PlatformScope() }
func ProjectScope(id string) Scope { return models.ProjectScope(id) }

type BaselinePolicy = models.BaselinePolicy

const (
	BaselineNone          = models.BaselineNone
	BaselineAdmin         = models.BaselineAdmin
	BaselineProjectMember = models.BaselineProjectMember
	BaselineAuthenticated = models.BaselineAuthenticated
)

type Definition = models.Definition
type Check = models.Check

type Assignment = models.Assignment
type RoleRule = models.RoleRule
type Role = models.Role
type RoleBinding = models.RoleBinding
type State = models.State

func NormalizeEmail(email string) string { return models.NormalizeEmail(email) }

var errNilDefinitions = errors.New("no definitions")

type Reason = evaluator.Reason

const (
	ReasonSystem          = evaluator.ReasonSystem
	ReasonAdministrator   = evaluator.ReasonAdministrator
	ReasonExplicitDeny    = evaluator.ReasonExplicitDeny
	ReasonExplicitAllow   = evaluator.ReasonExplicitAllow
	ReasonBaseline        = evaluator.ReasonBaseline
	ReasonDefaultDeny     = evaluator.ReasonDefaultDeny
	ReasonUnknownActor    = evaluator.ReasonUnknownActor
	ReasonInvalidCheck    = evaluator.ReasonInvalidCheck
	ReasonActorNotPresent = evaluator.ReasonActorNotPresent
)

type Decision = evaluator.Decision
