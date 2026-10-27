package presence

import "time"

// TTL is how long one heartbeat keeps a claim alive. It has to outlast the
// client's heartbeat interval by enough to ride out a dropped request, and
// stay short enough that a browser that dies without saying goodbye starts
// notifying its owner again quickly.
const TTL = 55 * time.Second

// maxClientsPerUser bounds one user's tracked clients and ordering tombstones.
// Once the cap is reached, the oldest report is retired first.
const maxClientsPerUser = 20

// clientPresence is one client's latest ordered report. Blank chat IDs are
// release tombstones: keeping them is what prevents a delayed older claim from
// reviving presence after the client has already left.
type clientPresence struct {
	chatID   string
	seenAt   time.Time
	revision uint64
}

func (p clientPresence) isLive(now time.Time) bool {
	return p.chatID != "" && p.seenAt.After(now.Add(-TTL))
}

// userPresence is one user's latest reports, keyed by client. Reports include
// both live claims and release tombstones.
type userPresence struct {
	byClient map[string]clientPresence
}

func newUserPresence() *userPresence {
	return &userPresence{byClient: map[string]clientPresence{}}
}

// update applies a client report only when it is newer than the last one seen.
// This makes concurrently arriving claims/releases deterministic.
func (p *userPresence) update(clientID, chatID string, revision uint64, now time.Time) {
	if current, known := p.byClient[clientID]; known && revision <= current.revision {
		return
	}
	if _, known := p.byClient[clientID]; !known && len(p.byClient) >= maxClientsPerUser {
		p.evictOldest()
	}
	p.byClient[clientID] = clientPresence{chatID: chatID, seenAt: now, revision: revision}
}

func (p *userPresence) isWatching(chatID string, now time.Time) bool {
	for _, client := range p.byClient {
		if client.chatID == chatID && client.isLive(now) {
			return true
		}
	}
	return false
}

func (p *userPresence) evictOldest() {
	var oldestID string
	var oldest time.Time
	for clientID, client := range p.byClient {
		if oldestID == "" || client.seenAt.Before(oldest) {
			oldestID, oldest = clientID, client.seenAt
		}
	}
	delete(p.byClient, oldestID)
}
