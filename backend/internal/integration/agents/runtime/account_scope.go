package runtime

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// AccountHomeScope returns a stable filesystem-safe name for one chat's
// provider account home. Including the execution scope prevents a host chat
// and project chat with the same conversation identifier from sharing state.
func AccountHomeScope(accountID, projectID, conversationID string) string {
	sum := sha256.Sum256([]byte(accountID + "\x00" + projectID + "\x00" + conversationID))
	return hex.EncodeToString(sum[:16])
}

// EmitForAccount stamps the saved account a run uses on every event the run
// emits, so what the provider reports about that account, such as its plan
// limits, is filed under it. An empty accountID, the provider's host login,
// leaves events unchanged.
func EmitForAccount(emit func(agent.Event), accountID string) func(agent.Event) {
	if accountID == "" {
		return emit
	}
	return func(ev agent.Event) {
		ev.AccountID = accountID
		emit(ev)
	}
}
