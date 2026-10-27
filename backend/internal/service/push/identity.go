package push

import "strings"

// NormalizeEmail is the identity form used as the subscription store key and
// for matching against project access lists.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
