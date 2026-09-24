// Package permission owns Remote's permission vocabulary, the persisted policy
// (direct assignments, custom roles, and role bindings), and the evaluator
// that answers whether an actor may perform a registered action.
//
// Definitions are code-owned: only developers add permission keys, and only by
// declaring a Definition in the service that owns the protected operation.
// Runtime callers can assign registered keys but can never invent one.
package permission

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Key is a stable, persisted permission identifier of the form
// <bounded-context>.<resource>.<action>. Renaming a key requires an explicit
// store migration or a temporary alias declared in code.
type Key string

// Effect is the outcome a matching rule contributes to a decision.
type Effect string

const (
	Allow Effect = "allow"
	Deny  Effect = "deny"
)

// Valid reports whether e is a known effect.
func (e Effect) Valid() bool { return e == Allow || e == Deny }

// ScopeKind names a typed resource scope. Add a new kind only when a real
// service needs one; there is deliberately no free-form attribute scope.
type ScopeKind string

const (
	ScopePlatform ScopeKind = "platform"
	ScopeProject  ScopeKind = "project"
)

// Scope is the resource a permission applies to. Matching is exact: platform
// matches only platform, and project:<id> matches only that project.
type Scope struct {
	Kind ScopeKind `json:"kind"`
	ID   string    `json:"id,omitempty"`
}

// PlatformScope is the scope of installation-wide actions.
func PlatformScope() Scope { return Scope{Kind: ScopePlatform} }

// ProjectScope is the scope of actions on one project.
func ProjectScope(id string) Scope { return Scope{Kind: ScopeProject, ID: id} }

var scopeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Validate rejects unknown kinds and IDs that do not fit their kind.
func (s Scope) Validate() error {
	switch s.Kind {
	case ScopePlatform:
		if s.ID != "" {
			return fmt.Errorf("%w: platform scope takes no id", ErrInvalidScope)
		}
	case ScopeProject:
		if !scopeIDPattern.MatchString(s.ID) {
			return fmt.Errorf("%w: malformed project id", ErrInvalidScope)
		}
	default:
		return fmt.Errorf("%w: unknown scope kind %q", ErrInvalidScope, s.Kind)
	}
	return nil
}

func (s Scope) String() string {
	if s.ID == "" {
		return string(s.Kind)
	}
	return string(s.Kind) + ":" + s.ID
}

// BaselinePolicy is the code-owned compatibility rule consulted when no
// explicit assignment matches. It preserves the access that existed before a
// permission was introduced.
type BaselinePolicy string

const (
	// BaselineNone grants nothing implicitly. It is also the default for a
	// definition that declares no policy.
	BaselineNone BaselinePolicy = "none"
	// BaselineAdmin admits current administrators only.
	BaselineAdmin BaselinePolicy = "admin"
	// BaselineProjectMember admits administrators and members of the checked
	// project.
	BaselineProjectMember BaselinePolicy = "project-member"
	// BaselineAuthenticated admits any registered actor.
	BaselineAuthenticated BaselinePolicy = "authenticated"
)

func (b BaselinePolicy) valid() bool {
	switch b {
	case BaselineNone, BaselineAdmin, BaselineProjectMember, BaselineAuthenticated:
		return true
	}
	return false
}

// Definition declares one registered permission.
type Definition struct {
	Key         Key
	Description string
	// Scopes lists the scope kinds the permission can be assigned and checked
	// at. Every listed kind is compared exactly, which is what makes a
	// Delegable definition safe to delegate.
	Scopes   []ScopeKind
	Baseline BaselinePolicy
	// Delegable marks permissions a non-administrator may hand to others while
	// holding them at the same scope.
	Delegable bool
}

// Check is one authorization question: may the actor use Permission on Scope.
type Check struct {
	Permission Key
	Scope      Scope
}

// keyPattern accepts exactly three lowercase dot-separated segments; each
// segment may contain digits and single interior hyphens.
var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*(\.[a-z][a-z0-9]*(-[a-z0-9]+)*){2}$`)

const maxKeyLength = 100

// Valid reports whether k is well formed. It does not consult the registry.
func (k Key) Valid() bool {
	return len(k) <= maxKeyLength && keyPattern.MatchString(string(k))
}

// SupportsScope reports whether the definition may be checked at kind.
func (d Definition) SupportsScope(kind ScopeKind) bool {
	for _, supported := range d.Scopes {
		if supported == kind {
			return true
		}
	}
	return false
}

func (d Definition) validate() error {
	if !d.Key.Valid() {
		return fmt.Errorf("%w: malformed key %q", ErrInvalidDefinition, d.Key)
	}
	if strings.TrimSpace(d.Description) == "" {
		return fmt.Errorf("%w: %s has no description", ErrInvalidDefinition, d.Key)
	}
	if len(d.Scopes) == 0 {
		return fmt.Errorf("%w: %s declares no scopes", ErrInvalidDefinition, d.Key)
	}
	seen := make(map[ScopeKind]struct{}, len(d.Scopes))
	for _, kind := range d.Scopes {
		if err := (Scope{Kind: kind, ID: probeID(kind)}).Validate(); err != nil {
			return fmt.Errorf("%w: %s: %w", ErrInvalidDefinition, d.Key, err)
		}
		if _, dup := seen[kind]; dup {
			return fmt.Errorf("%w: %s repeats scope %q", ErrInvalidDefinition, d.Key, kind)
		}
		seen[kind] = struct{}{}
	}
	if !d.Baseline.valid() {
		return fmt.Errorf("%w: %s has unknown baseline %q", ErrInvalidDefinition, d.Key, d.Baseline)
	}
	if d.Baseline == BaselineProjectMember && (len(d.Scopes) != 1 || d.Scopes[0] != ScopeProject) {
		return fmt.Errorf("%w: %s uses the project-member baseline, which requires exactly the project scope",
			ErrInvalidDefinition, d.Key)
	}
	return nil
}

// probeID returns an ID that is valid for kind, so a kind can be checked
// without a concrete resource.
func probeID(kind ScopeKind) string {
	if kind == ScopePlatform {
		return ""
	}
	return "probe"
}

var errNilDefinitions = errors.New("no definitions")
