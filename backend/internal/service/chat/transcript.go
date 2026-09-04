package chat

import (
	"context"
	"strconv"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

// TranscriptEventSource exposes storage-order events without making the
// repository responsible for transcript projection policy.
type TranscriptEventSource interface {
	ScanEvents(ctx context.Context, id ID, visit func(Event)) error
}

// WithTranscriptEventSource supplies the ordered event stream used to build
// transcript pages.
func WithTranscriptEventSource(source TranscriptEventSource) Option {
	return func(service *Service) {
		service.transcriptEvents = source
	}
}

// TranscriptPageQuery pages the user-visible transcript in whole turns. The
// sequence cursor remains tied to the raw event log so existing chats need no
// migration and live replay can keep its event-based contract.
type TranscriptPageQuery struct {
	Limit     int
	BeforeSeq int64
}

// TranscriptTurn is a read projection of one prompt run. Events retain the
// existing transport shape, but adjacent streaming deltas are compacted and a
// page never begins inside the turn.
type TranscriptTurn struct {
	ID       string  `json:"id"`
	StartSeq int64   `json:"startSeq"`
	EndSeq   int64   `json:"endSeq"`
	Events   []Event `json:"events"`
}

// TranscriptPage is the bounded turn projection returned to history clients.
type TranscriptPage struct {
	Turns      []TranscriptTurn `json:"turns"`
	NextBefore int64            `json:"nextBefore,omitempty"`
	LastSeq    int64            `json:"lastSeq"`
	HasMore    bool             `json:"hasMore"`
}

// TranscriptPage projects the raw append-only stream into complete prompt
// turns for history rendering. Raw events remain authoritative for live replay;
// this read model only changes the unit used for backward pagination.
func (s *Service) TranscriptPage(
	ctx context.Context,
	id ID,
	query TranscriptPageQuery,
) (TranscriptPage, error) {
	if !ValidID(id) {
		return TranscriptPage{}, ErrInvalidID
	}

	projection := newTranscriptProjection(query)
	err := s.transcriptEvents.ScanEvents(ctx, id, projection.visit)
	if err != nil {
		return TranscriptPage{}, err
	}
	return projection.page(), nil
}

type transcriptProjection struct {
	beforeSeq int64
	limit     int
	lastSeq   int64
	hasMore   bool
	turns     []TranscriptTurn
	current   transcriptTurnBuffer
}

func newTranscriptProjection(query TranscriptPageQuery) *transcriptProjection {
	limit := query.Limit
	if limit <= 0 {
		limit = configconstants.DefaultChatTranscriptTurnLimit
	}
	if limit > configconstants.MaxChatTranscriptTurnLimit {
		limit = configconstants.MaxChatTranscriptTurnLimit
	}
	return &transcriptProjection{
		beforeSeq: query.BeforeSeq,
		limit:     limit,
		turns:     make([]TranscriptTurn, 0, limit),
	}
}

func (projection *transcriptProjection) visit(event Event) {
	if event.Seq > projection.lastSeq {
		projection.lastSeq = event.Seq
	}
	if projection.beforeSeq > 0 && event.Seq >= projection.beforeSeq {
		return
	}
	if projection.current.startsNewTurn(event) {
		projection.flushCurrent()
	}
	projection.current.append(event)
}

func (projection *transcriptProjection) flushCurrent() {
	if len(projection.current.events) == 0 {
		return
	}
	turn := projection.current.project()
	if len(projection.turns) == projection.limit {
		copy(projection.turns, projection.turns[1:])
		projection.turns[len(projection.turns)-1] = turn
		projection.hasMore = true
	} else {
		projection.turns = append(projection.turns, turn)
	}
	projection.current = transcriptTurnBuffer{}
}

func (projection *transcriptProjection) page() TranscriptPage {
	projection.flushCurrent()
	var nextBefore int64
	if projection.hasMore && len(projection.turns) > 0 {
		nextBefore = projection.turns[0].StartSeq
	}
	return TranscriptPage{
		Turns:      projection.turns,
		NextBefore: nextBefore,
		LastSeq:    projection.lastSeq,
		HasMore:    projection.hasMore,
	}
}

type transcriptTurnBuffer struct {
	id       string
	startSeq int64
	endSeq   int64
	hasUser  bool
	events   []Event
}

func (turn *transcriptTurnBuffer) startsNewTurn(ev Event) bool {
	if len(turn.events) == 0 {
		return false
	}
	if ev.TurnID != "" {
		if turn.id != "" {
			return turn.id != ev.TurnID
		}
		return turn.hasUser
	}
	return ev.Type == "user" && (turn.hasUser || turn.id != "")
}

func (turn *transcriptTurnBuffer) append(ev Event) {
	if len(turn.events) == 0 {
		turn.startSeq = ev.Seq
	}
	turn.endSeq = ev.Seq
	if turn.id == "" && ev.TurnID != "" {
		turn.id = ev.TurnID
	}
	if ev.Type == "user" {
		turn.hasUser = true
	}

	lastIndex := len(turn.events) - 1
	if lastIndex >= 0 && transcriptEventsCanCoalesce(turn.events[lastIndex], ev) {
		turn.events[lastIndex].Text += ev.Text
		return
	}
	turn.events = append(turn.events, ev)
}

func (turn transcriptTurnBuffer) project() TranscriptTurn {
	id := turn.id
	if id == "" {
		id = "legacy-" + strconv.FormatInt(turn.startSeq, 10)
	}
	return TranscriptTurn{
		ID:       id,
		StartSeq: turn.startSeq,
		EndSeq:   turn.endSeq,
		Events:   turn.events,
	}
}

func transcriptEventsCanCoalesce(left, right Event) bool {
	if left.Type != right.Type || (left.Type != "assistant_text" && left.Type != "thinking") {
		return false
	}
	return left.TurnID == right.TurnID &&
		left.MessageID == right.MessageID &&
		left.Provider == right.Provider &&
		left.ScheduledTaskID == right.ScheduledTaskID
}
