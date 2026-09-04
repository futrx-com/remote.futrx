package chat

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

type transcriptRepository struct {
	Repository
	events  []Event
	scanErr error
	scans   int
}

func (r *transcriptRepository) ScanEvents(
	ctx context.Context,
	_ ID,
	visit func(Event),
) error {
	r.scans++
	for _, event := range r.events {
		if err := ctx.Err(); err != nil {
			return err
		}
		visit(event)
	}
	return r.scanErr
}

func TestTranscriptPagePagesWholeTurnsAndCompactsDeltas(t *testing.T) {
	events := []Event{
		{T: 1, Type: "user", Text: "older question"},
		{T: 2, Type: "assistant_text", Text: "older answer"},
		{T: 3, Type: "complete"},
		{T: 4, Type: "user", TurnID: "turn-new", Text: "new question"},
	}
	for i := 0; i < 300; i++ {
		events = append(events, Event{
			T:         int64(5 + i),
			Type:      "assistant_text",
			TurnID:    "turn-new",
			MessageID: "message-new",
			Text:      "x",
		})
	}
	events = append(events, Event{T: 305, Type: "complete", TurnID: "turn-new"})
	assignEventSequences(events)

	service := newTranscriptService(events)
	page, err := service.TranscriptPage(
		context.Background(),
		"abcd",
		TranscriptPageQuery{Limit: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 1 {
		t.Fatalf("latest transcript page = %#v", page)
	}
	turn := page.Turns[0]
	if turn.ID != "turn-new" || turn.StartSeq != 4 || turn.EndSeq != 305 {
		t.Fatalf("latest turn boundaries = %#v", turn)
	}
	if len(turn.Events) != 3 || turn.Events[0].Type != "user" ||
		turn.Events[1].Type != "assistant_text" || turn.Events[2].Type != "complete" {
		t.Fatalf("compacted turn events = %#v", turn.Events)
	}
	if turn.Events[1].Text != strings.Repeat("x", 300) {
		t.Fatalf("assistant text length = %d, want 300", len(turn.Events[1].Text))
	}
	if !page.HasMore || page.NextBefore != 4 || page.LastSeq != 305 {
		t.Fatalf("latest transcript cursors = %#v", page)
	}

	older, err := service.TranscriptPage(
		context.Background(),
		"abcd",
		TranscriptPageQuery{Limit: 1, BeforeSeq: page.NextBefore},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Turns) != 1 || older.Turns[0].StartSeq != 1 || older.Turns[0].EndSeq != 3 {
		t.Fatalf("older transcript page = %#v", older)
	}
	if older.HasMore || older.NextBefore != 0 || older.LastSeq != 305 {
		t.Fatalf("older transcript cursors = %#v", older)
	}
}

func TestTranscriptPageKeepsIncompleteTurnAndToolLifecycleTogether(t *testing.T) {
	events := []Event{
		{T: 1, Type: "user", TurnID: "turn-old", Text: "old"},
		{T: 2, Type: "assistant_text", TurnID: "turn-old", Text: "done"},
		{T: 3, Type: "complete", TurnID: "turn-old"},
		{T: 4, Type: "user", TurnID: "turn-live", Text: "inspect"},
		{T: 5, Type: "assistant_text", TurnID: "turn-live", Text: "before"},
		{T: 6, Type: "tool_use_start", TurnID: "turn-live", ID: "tool-1", Name: "Bash"},
	}
	for i := 0; i < 260; i++ {
		events = append(events, Event{
			T:         int64(7 + i),
			Type:      "thinking",
			TurnID:    "turn-live",
			MessageID: "reasoning-1",
			Text:      "r",
		})
	}
	events = append(events,
		Event{T: 267, Type: "tool_use_end", TurnID: "turn-live", ID: "tool-1", Output: "ok"},
		Event{T: 268, Type: "assistant_text", TurnID: "turn-live", Text: "after"},
	)
	assignEventSequences(events)

	page, err := newTranscriptService(events).TranscriptPage(
		context.Background(),
		"abcd",
		TranscriptPageQuery{Limit: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 1 || page.Turns[0].ID != "turn-live" {
		t.Fatalf("incomplete page = %#v", page)
	}
	projected := page.Turns[0].Events
	if len(projected) != 6 {
		t.Fatalf("incomplete compacted events = %#v", projected)
	}
	if projected[2].Type != "tool_use_start" || projected[2].ID != "tool-1" ||
		projected[3].Type != "thinking" || projected[3].Text != strings.Repeat("r", 260) ||
		projected[4].Type != "tool_use_end" || projected[4].ID != "tool-1" || projected[4].Output != "ok" {
		t.Fatalf("tool lifecycle was not preserved: %#v", projected)
	}
	if projected[len(projected)-1].Type != "assistant_text" || projected[len(projected)-1].Text != "after" {
		t.Fatalf("incomplete assistant tail = %#v", projected[len(projected)-1])
	}
	if !page.HasMore || page.NextBefore != 4 {
		t.Fatalf("incomplete page cursor = %#v", page)
	}
}

func TestTranscriptPageTreatsLegacyOrphanEventsAsOneSafePage(t *testing.T) {
	events := make([]Event, 300)
	for i := range events {
		events[i] = Event{
			Seq:  int64(i + 1),
			T:    int64(i + 1),
			Type: "assistant_text",
			Text: "x",
		}
	}

	page, err := newTranscriptService(events).TranscriptPage(
		context.Background(),
		"abcd",
		TranscriptPageQuery{Limit: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 1 || len(page.Turns[0].Events) != 1 ||
		page.Turns[0].Events[0].Text != strings.Repeat("x", 300) {
		t.Fatalf("legacy orphan transcript = %#v", page)
	}
	if page.HasMore || page.NextBefore != 0 {
		t.Fatalf("orphan transcript must not expose an unsafe cursor: %#v", page)
	}
}

func TestTranscriptPageNormalizesTurnLimits(t *testing.T) {
	events := make([]Event, 105)
	for i := range events {
		events[i] = Event{
			Seq:    int64(i + 1),
			T:      int64(i + 1),
			Type:   "user",
			TurnID: "turn-" + strconv.Itoa(i+1),
			Text:   "prompt",
		}
	}

	tests := []struct {
		name           string
		limit          int
		wantTurns      int
		wantNextBefore int64
	}{
		{name: "default", limit: 0, wantTurns: 20, wantNextBefore: 86},
		{name: "maximum", limit: 101, wantTurns: 100, wantNextBefore: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := newTranscriptService(events).TranscriptPage(
				context.Background(),
				"abcd",
				TranscriptPageQuery{Limit: tt.limit},
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Turns) != tt.wantTurns || !page.HasMore ||
				page.NextBefore != tt.wantNextBefore || page.LastSeq != 105 {
				t.Fatalf("normalized page = %#v", page)
			}
		})
	}
}

func TestTranscriptPagePreservesValidationAndScanErrors(t *testing.T) {
	scanErr := errors.New("scan failed")
	repository := &transcriptRepository{scanErr: scanErr}
	service := New(repository, nil, nil, nil, WithTranscriptEventSource(repository))

	if _, err := service.TranscriptPage(
		context.Background(),
		"abc",
		TranscriptPageQuery{},
	); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("invalid id error = %v, want %v", err, ErrInvalidID)
	}
	if repository.scans != 0 {
		t.Fatalf("invalid id reached repository %d times", repository.scans)
	}

	if _, err := service.TranscriptPage(
		context.Background(),
		"abcd",
		TranscriptPageQuery{},
	); !errors.Is(err, scanErr) {
		t.Fatalf("scan error = %v, want %v", err, scanErr)
	}
}

func TestTranscriptEventsCoalesceOnlyWhenIdentityMatches(t *testing.T) {
	base := Event{
		Type:            "assistant_text",
		TurnID:          "turn-1",
		MessageID:       "message-1",
		Provider:        ProviderCodex,
		ScheduledTaskID: "task-1",
	}
	tests := []struct {
		name  string
		left  Event
		right Event
		want  bool
	}{
		{name: "assistant identity", left: base, right: base, want: true},
		{
			name:  "thinking identity",
			left:  Event{Type: "thinking", TurnID: "turn-1", MessageID: "message-1"},
			right: Event{Type: "thinking", TurnID: "turn-1", MessageID: "message-1"},
			want:  true,
		},
		{name: "event type", left: base, right: withTranscriptEvent(base, func(event *Event) { event.Type = "thinking" })},
		{name: "unsupported type", left: Event{Type: "user"}, right: Event{Type: "user"}},
		{name: "turn", left: base, right: withTranscriptEvent(base, func(event *Event) { event.TurnID = "turn-2" })},
		{name: "message", left: base, right: withTranscriptEvent(base, func(event *Event) { event.MessageID = "message-2" })},
		{name: "provider", left: base, right: withTranscriptEvent(base, func(event *Event) { event.Provider = ProviderClaude })},
		{name: "scheduled task", left: base, right: withTranscriptEvent(base, func(event *Event) { event.ScheduledTaskID = "task-2" })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := transcriptEventsCanCoalesce(tt.left, tt.right); got != tt.want {
				t.Fatalf("transcriptEventsCanCoalesce() = %t, want %t", got, tt.want)
			}
		})
	}
}

func newTranscriptService(events []Event) *Service {
	repository := &transcriptRepository{events: events}
	return New(repository, nil, nil, nil, WithTranscriptEventSource(repository))
}

func assignEventSequences(events []Event) {
	for i := range events {
		events[i].Seq = int64(i + 1)
	}
}

func withTranscriptEvent(event Event, update func(*Event)) Event {
	update(&event)
	return event
}
