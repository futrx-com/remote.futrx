// Package presence tracks which chat each user is looking at right now.
//
// It exists because a service worker can only silence the device it runs on:
// with no shared signal, a phone still buzzes about a question its owner is
// reading on a laptop. Clients heartbeat what they are watching, and the push
// notifier skips a user whose own eyes are already on the chat.
//
// State is in-memory and expires on its own. It is a hint about this moment,
// nothing worth persisting, and the cost of losing it is one extra
// notification rather than a missed one.
package presence

import (
	"sync"
	"time"
)

// Service records which chat each of a user's clients is watching.
//
// Claims are tracked per client rather than per user so that clients stay
// independent: a background tab signing off cannot cancel the claim of the
// focused tab beside it, whichever order their requests land in.
type Service struct {
	clock func() time.Time

	mu    sync.Mutex
	users map[string]*userPresence
}

// Report is one client's ordered statement of what it currently has on
// screen. A blank ChatID is a release tombstone.
type Report struct {
	ClientID string
	ChatID   string
	Revision uint64
}

func New() *Service {
	return newAt(time.Now)
}

// newAt builds a service reading a caller-supplied clock, so tests can age
// claims out without sleeping through the real TTL.
func newAt(clock func() time.Time) *Service {
	return &Service{clock: clock, users: map[string]*userPresence{}}
}

// Record applies one ordered client report. Blank chat IDs are retained as
// release tombstones so a delayed older claim cannot revive stale presence.
func (s *Service) Record(email string, report Report) {
	if s == nil {
		return
	}
	email = normalizeEmail(email)
	report.ClientID = clientKey(report.ClientID)
	report.ChatID = trim(report.ChatID)
	if email == "" || report.Revision == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	presence := s.users[email]
	if presence == nil {
		presence = newUserPresence()
		s.users[email] = presence
	}
	presence.update(report.ClientID, report.ChatID, report.Revision, s.clock())
}

// IsWatching reports whether any of a user's live clients has this chat on
// screen.
func (s *Service) IsWatching(email, chatID string) bool {
	if s == nil {
		return false
	}
	email, chatID = normalizeEmail(email), trim(chatID)
	if email == "" || chatID == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	presence := s.users[email]
	return presence != nil && presence.isWatching(chatID, s.clock())
}

// Filter drops the recipients who are already watching this chat. A nil
// service filters nothing, so a deployment without presence tracking notifies
// exactly as it did before.
func (s *Service) Filter(recipients []string, chatID string) []string {
	if s == nil || len(recipients) == 0 || trim(chatID) == "" {
		return recipients
	}
	kept := make([]string, 0, len(recipients))
	for _, email := range recipients {
		if s.IsWatching(email, chatID) {
			continue
		}
		kept = append(kept, email)
	}
	return kept
}
