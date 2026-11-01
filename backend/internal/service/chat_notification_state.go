package service

import (
	"strconv"
	"strings"
	"sync"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// chatNotificationState owns in-memory question tracking and delivery claims.
// Its lock never covers repository reads, presence checks, or push delivery.
type chatNotificationState struct {
	// parked records unanswered requests. Legacy runs can end while waiting;
	// native requests resume inside the same run. Routine terminal events must
	// not replace a question that still needs the user.
	mu     sync.Mutex
	parked map[servicechat.ID]servicechat.QuestionState

	questionBatches map[servicechat.ID]map[questionBatch]uint64

	// notified records, per chat, the chat's read marker at the moment its
	// last notification went out. Until that marker moves forward the user
	// has not seen the chat since, so later turns stay quiet instead of
	// piling up: iOS ignores the per-chat tag and stacks every notification.
	// Held in memory; a restart costs at most one extra notification per chat.
	notified map[servicechat.ID]unreadNotice
	claims   uint64
}

// unreadNotice is one chat's outstanding notification. claim identifies the
// send that recorded it, so a failed delivery can undo only its own record.
type unreadNotice struct {
	readAt int64
	claim  uint64
}

type questionBatch struct {
	turnID    string
	eventType string
	id        string
}

func questionBatchFromEvent(event servicechat.Event) questionBatch {
	id := event.InteractionID
	if id == "" {
		id = event.ID
	}
	if id == "" {
		id = strconv.FormatInt(event.Seq, 10)
	}
	return questionBatch{turnID: event.TurnID, eventType: event.Type, id: id}
}

// claimUnreadSlot reports whether the chat may notify again and, if so,
// records the read marker the notification is about to go out against. A
// distinct question batch bypasses unread suppression: an older "finished"
// notification must not hide a fresh request. Duplicate batches stay quiet.
//
// The slot is claimed before delivery so two events racing through here
// cannot both send; releaseUnreadSlot hands it back if nothing arrived.
func (s *chatNotificationState) claimUnreadSlot(
	chatID servicechat.ID,
	lastReadAt int64,
	isQuestion bool,
	batch questionBatch,
) (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.notified == nil {
		s.notified = map[servicechat.ID]unreadNotice{}
	}
	if isQuestion {
		if s.questionBatches == nil {
			s.questionBatches = map[servicechat.ID]map[questionBatch]uint64{}
		}
		if s.questionBatches[chatID] == nil {
			s.questionBatches[chatID] = map[questionBatch]uint64{}
		}
		if _, sent := s.questionBatches[chatID][batch]; sent {
			return 0, false
		}
	}
	if notice, pending := s.notified[chatID]; pending && lastReadAt <= notice.readAt && !isQuestion {
		return 0, false
	}
	s.claims++
	s.notified[chatID] = unreadNotice{readAt: lastReadAt, claim: s.claims}
	if isQuestion {
		s.questionBatches[chatID][batch] = s.claims
	}
	return s.claims, true
}

// releaseUnreadSlot forgets a claim whose notification reached no device, so
// the chat is not kept quiet about something the user never saw. A newer
// claim for the same chat is left alone.
func (s *chatNotificationState) releaseUnreadSlot(chatID servicechat.ID, claim uint64, batch questionBatch) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if notice, ok := s.notified[chatID]; ok && notice.claim == claim {
		delete(s.notified, chatID)
	}
	if s.questionBatches[chatID][batch] == claim {
		delete(s.questionBatches[chatID], batch)
	}
}

// trackUserPrompt treats a prompt the user typed as having seen the chat, so
// the run it starts may notify again. An application or legacy scheduled prompt
// does not indicate that the user has seen the chat and leaves it unread.
func (s *chatNotificationState) trackUserPrompt(chatID servicechat.ID, event servicechat.Event) {
	if event.Type != "user" || strings.TrimSpace(event.ScheduledTaskID) != "" || event.ApplicationID != "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.notified, chatID)
}

// trackParkedRun maintains the "waiting on an answer" flag and reports whether
// the event should still be allowed to notify. Legacy questions survive
// completion until answered; native questions clear on matching resolutions.
func (s *chatNotificationState) trackParkedRun(
	chatID servicechat.ID,
	event servicechat.Event,
) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.parked == nil {
		s.parked = map[servicechat.ID]servicechat.QuestionState{}
	}
	state := s.parked[chatID]
	if state.AnsweredBy(event) {
		// Structured answers resume the same run rather than appending a user
		// prompt. They still acknowledge the question notification.
		delete(s.notified, chatID)
	}
	if event.Type == "user" {
		delete(s.questionBatches, chatID)
	}
	if (event.Type == "complete" || event.Type == "error") && state.AwaitingAnswer(true) &&
		(event.TurnID == "" || state.TurnID == "" || event.TurnID == state.TurnID) {
		if event.Type == "error" && event.Status == "retrying" {
			// Recovery keeps the question actionable and must not replace its alert.
			return false
		}
		if event.Type == "complete" && state.LegacyPending {
			s.parked[chatID], _ = state.Apply(event)
		} else {
			delete(s.parked, chatID)
		}
		return false
	}
	next, _ := state.Apply(event)
	if next.AwaitingAnswer(true) {
		s.parked[chatID] = next
	} else {
		delete(s.parked, chatID)
	}
	return true
}

func (s *chatNotificationState) deleteChat(chatID servicechat.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.parked, chatID)
	delete(s.questionBatches, chatID)
	delete(s.notified, chatID)
}
