package runtime

import (
	"crypto/sha256"
	"encoding/hex"
)

// AccountHomeScope returns a stable filesystem-safe name for one chat's
// provider account home. Including the execution scope prevents a host chat
// and project chat with the same conversation identifier from sharing state.
func AccountHomeScope(accountID, projectID, conversationID string) string {
	sum := sha256.Sum256([]byte(accountID + "\x00" + projectID + "\x00" + conversationID))
	return hex.EncodeToString(sum[:16])
}
