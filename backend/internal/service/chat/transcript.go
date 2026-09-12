package chat

import (
	"context"
	"errors"
	"strconv"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

// TranscriptEventWindow contains enough whole turns to determine a page and
// whether an older page exists. LastSeq describes the complete event stream,
// not only the returned window.
type TranscriptEventWindow struct {
	Events  []Event
	LastSeq int64
}

// WithTranscriptEventSource supplies the ordered event stream used to build
// transcript pages.
func WithTranscriptEventSource(source TranscriptEventSource) Option {
	return func(service *Service) {
		service.transcriptEvents = source
	}
}

// WithTranscriptEventWindowSource supplies the optimized event-window reader
// explicitly, independently of the canonical event scanner.
func WithTranscriptEventWindowSource(source TranscriptEventWindowSource) Option {
	return func(service *Service) {
		service.transcriptWindow = source
	}
}

// TranscriptPageQuery pages the user-visible transcript in whole turns. The
// sequence cursor remains tied to the raw event log so existing chats need no
// migration and live replay can keep its event-based contract.
type TranscriptPageQuery struct {
	Limit     int
	BeforeSeq int64
	ByteLimit int
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
	Turns      []TranscriptTurn         `json:"turns"`
	NextBefore int64                    `json:"nextBefore,omitempty"`
	LastSeq    int64                    `json:"lastSeq"`
	HasMore    bool                     `json:"hasMore"`
	Indexing   *TranscriptIndexProgress `json:"indexing,omitempty"`
}

// TranscriptIndexProgress lets clients keep the live stream connected while
// a legacy JSONL file is projected in resumable background batches.
type TranscriptIndexProgress struct {
	IndexedBytes int64 `json:"indexedBytes"`
	TotalBytes   int64 `json:"totalBytes"`
	TailSeqKnown bool  `json:"tailSeqKnown"`
}

// TranscriptContentPage is one valid UTF-8 chunk of an oversized transcript
// field. ContentID is opaque and scoped to the chat by the service boundary.
type TranscriptContentPage struct {
	ContentID  string `json:"contentId"`
	Content    string `json:"content"`
	NextAfter  int64  `json:"nextAfter,omitempty"`
	TotalBytes int64  `json:"totalBytes"`
	Complete   bool   `json:"complete"`
}

// WithTranscriptProjectionSource selects the compact durable read model when
// storage provides it, while retaining the scanner/window fallback for tests
// and recovery.
func WithTranscriptProjectionSource(source TranscriptProjectionSource) Option {
	return func(service *Service) {
		service.transcriptProjection = source
	}
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
	if s.transcriptProjection != nil {
		page, err := s.transcriptProjection.ReadTranscriptPage(ctx, id, query)
		if err == nil || !errors.Is(err, ErrTranscriptProjectionUnavailable) {
			return page, err
		}
	}

	projection := newTranscriptProjection(query)
	if s.transcriptWindow != nil {
		window, err := s.transcriptWindow.ReadTranscriptEventWindow(
			ctx,
			id,
			projection.beforeSeq,
			projection.limit,
		)
		if err != nil {
			return TranscriptPage{}, err
		}
		projection.lastSeq = window.LastSeq
		for _, event := range window.Events {
			projection.visit(event)
		}
		return projection.page(), nil
	}
	err := s.transcriptEvents.ScanEvents(ctx, id, projection.visit)
	if err != nil {
		return TranscriptPage{}, err
	}
	return projection.page(), nil
}

// TranscriptContent retrieves an oversized projected field without exposing a
// filesystem path or requiring the client to download its containing event.
func (s *Service) TranscriptContent(
	ctx context.Context,
	id ID,
	contentID string,
	afterBytes int64,
	limitBytes int,
) (TranscriptContentPage, error) {
	if !ValidID(id) {
		return TranscriptContentPage{}, ErrInvalidID
	}
	if s.transcriptProjection == nil {
		return TranscriptContentPage{}, ErrTranscriptProjectionUnavailable
	}
	return s.transcriptProjection.ReadTranscriptContent(
		ctx, id, contentID, afterBytes, limitBytes,
	)
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
	return EventStartsTranscriptTurn(turn.id, turn.hasUser, len(turn.events) > 0, ev)
}

// EventStartsTranscriptTurn is the shared boundary rule for transcript
// projection and derived storage indexes. Keeping it here prevents an index
// migration from silently disagreeing with the user-visible projection.
func EventStartsTranscriptTurn(
	currentTurnID string,
	currentHasUser bool,
	currentHasEvents bool,
	event Event,
) bool {
	if !currentHasEvents {
		return false
	}
	if event.TurnID != "" {
		if currentTurnID != "" {
			return currentTurnID != event.TurnID
		}
		return currentHasUser
	}
	return event.Type == "user" && (currentHasUser || currentTurnID != "")
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
