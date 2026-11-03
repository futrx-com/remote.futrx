package user

import "strings"

type Role string

// RoleAdmin is the only base role. A registered user without it holds the
// zero Role and gets their access from custom permission roles.
const RoleAdmin Role = "admin"

// NormalizeRole maps input to a Role. Unknown values are returned as-is so
// ValidRole rejects them; the legacy "member" value maps to the zero Role.
func NormalizeRole(r string) Role {
	s := strings.ToLower(strings.TrimSpace(r))
	switch s {
	case string(RoleAdmin):
		return RoleAdmin
	case "", "member":
		return ""
	default:
		return Role(s)
	}
}

func ValidRole(r Role) bool {
	return r == RoleAdmin || r == ""
}

// NormalizeEmail lowercases + trims. Used everywhere to compare emails
// case-insensitively; OAuth gives us lowercase but humans typing into the
// admin UI may not.
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// User is one registered email. AddedAt is unix millis; AddedBy is the
// email of the admin who added them (empty for the bootstrap admin).
type User struct {
	Email   string `json:"email"`
	Role    Role   `json:"role"`
	AddedAt int64  `json:"addedAt"`
	AddedBy string `json:"addedBy,omitempty"`
}
