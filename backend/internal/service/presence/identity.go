package presence

import "strings"

// clientKey addresses one client. A caller that sends no id still gets a slot
// — as a single implicit client — rather than being silently untracked, and an
// oversized id is cut down so an opaque string cannot set the key size.
func clientKey(clientID string) string {
	clientID = trim(clientID)
	if clientID == "" {
		return "-"
	}
	if len(clientID) > 128 {
		return clientID[:128]
	}
	return clientID
}

// normalizeEmail matches the identity form the push subscription store keys
// on, so a presence claim and a subscription resolve to the same user.
func normalizeEmail(email string) string {
	return strings.ToLower(trim(email))
}

func trim(value string) string {
	return strings.TrimSpace(value)
}
