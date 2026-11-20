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

// TranscriptPageQuery bounds a history page by turn count and bytes. A large
// turn may span pages; the cursor remains a sequence in the saved event stream.
type TranscriptPageQuery struct {
	Limit     int
	BeforeSeq int64
	ByteLimit int
}

// TranscriptTurn groups the events selected for one prompt run. Events retain
// their transport shape; a byte-limited page may contain only part of a turn.
type TranscriptTurn struct {
	ID       string  `json:"id"`
	StartSeq int64   `json:"startSeq"`
	EndSeq   int64   `json:"endSeq"`
	Events   []Event `json:"events"`
}

// TranscriptPage contains the requested events grouped into turns for display.
type TranscriptPage struct {
	Turns      []TranscriptTurn `json:"turns"`
	NextBefore int64            `json:"nextBefore,omitempty"`
	LastSeq    int64            `json:"lastSeq"`
	HasMore    bool             `json:"hasMore"`
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

// WithTranscriptReader supplies the indexed page and content reader. When no
// reader is configured, pages are assembled from the event window or scanner.
func WithTranscriptReader(source TranscriptReader) Option {
	return func(service *Service) {
		service.transcriptReader = source
	}
}

// TranscriptPage reads saved events as a bounded page grouped into prompt
// turns. The underlying event sequences also remain the live replay cursors.
func (s *Service) TranscriptPage(
	ctx context.Context,
	id ID,
	query TranscriptPageQuery,
) (TranscriptPage, error) {
	if !ValidID(id) {
		return TranscriptPage{}, ErrInvalidID
	}
	if s.transcriptReader != nil {
		page, err := s.transcriptReader.ReadTranscriptPage(ctx, id, query)
		if err == nil || !errors.Is(err, ErrTranscriptUnavailable) {
			return page, err
		}
	}

	assembler := newTranscriptPageAssembler(query)
	if s.transcriptWindow != nil {
		window, err := s.transcriptWindow.ReadTranscriptEventWindow(
			ctx,
			id,
			assembler.beforeSeq,
			assembler.limit,
		)
		if err != nil {
			return TranscriptPage{}, err
		}
		assembler.lastSeq = window.LastSeq
		for _, event := range window.Events {
			assembler.visit(event)
		}
		return assembler.page(), nil
	}
	err := s.transcriptEvents.ScanEvents(ctx, id, assembler.visit)
	if err != nil {
		return TranscriptPage{}, err
	}
	return assembler.page(), nil
}

// TranscriptContent retrieves an oversized event field without exposing a
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
	if s.transcriptReader == nil {
		return TranscriptContentPage{}, ErrTranscriptUnavailable
	}
	return s.transcriptReader.ReadTranscriptContent(
		ctx, id, contentID, afterBytes, limitBytes,
	)
}

type transcriptPageAssembler struct {
	beforeSeq int64
	limit     int
	lastSeq   int64
	hasMore   bool
	turns     []TranscriptTurn
	current   transcriptTurnBuffer
}

func newTranscriptPageAssembler(query TranscriptPageQuery) *transcriptPageAssembler {
	limit := query.Limit
	if limit <= 0 {
		limit = configconstants.DefaultChatTranscriptTurnLimit
	}
	if limit > configconstants.MaxChatTranscriptTurnLimit {
		limit = configconstants.MaxChatTranscriptTurnLimit
	}
	return &transcriptPageAssembler{
		beforeSeq: query.BeforeSeq,
		limit:     limit,
		turns:     make([]TranscriptTurn, 0, limit),
	}
}

func (assembler *transcriptPageAssembler) visit(event Event) {
	if event.Seq > assembler.lastSeq {
		assembler.lastSeq = event.Seq
	}
	if assembler.beforeSeq > 0 && event.Seq >= assembler.beforeSeq {
		return
	}
	if assembler.current.startsNewTurn(event) {
		assembler.flushCurrent()
	}
	assembler.current.append(event)
}

func (assembler *transcriptPageAssembler) flushCurrent() {
	if len(assembler.current.events) == 0 {
		return
	}
	turn := assembler.current.project()
	if len(assembler.turns) == assembler.limit {
		copy(assembler.turns, assembler.turns[1:])
		assembler.turns[len(assembler.turns)-1] = turn
		assembler.hasMore = true
	} else {
		assembler.turns = append(assembler.turns, turn)
	}
	assembler.current = transcriptTurnBuffer{}
}

func (assembler *transcriptPageAssembler) page() TranscriptPage {
	assembler.flushCurrent()
	var nextBefore int64
	if assembler.hasMore && len(assembler.turns) > 0 {
		nextBefore = assembler.turns[0].StartSeq
	}
	return TranscriptPage{
		Turns:      assembler.turns,
		NextBefore: nextBefore,
		LastSeq:    assembler.lastSeq,
		HasMore:    assembler.hasMore,
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
		left.ScheduledTaskID == right.ScheduledTaskID &&
		left.ApplicationID == right.ApplicationID && left.ApplicationRequestID == right.ApplicationRequestID
}
