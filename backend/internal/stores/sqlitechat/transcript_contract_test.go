package sqlitechat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func TestTranscriptCollapsesTelemetryAndLoadsFullResponses(t *testing.T) {
	toolOutput := strings.Repeat("tool-result-🙂\n", 4_000)
	subagentOutput := strings.Repeat("subagent-tool-output\n", 3_000)
	subagentMessage := strings.Repeat("final subagent report\n", 3_000)
	firstCollaboration := json.RawMessage(`{
		"type":"subagentThread",
		"tools":[{"id":"child-tool","name":"exec","output":"working"}],
		"agentsStates":{"child":{"status":"inProgress"}}
	}`)
	finalCollaboration, err := json.Marshal(map[string]any{
		"type": "subagentThread",
		"tools": []any{map[string]any{
			"id": "child-tool", "name": "exec", "status": "completed",
			"output": subagentOutput,
		}},
		"agentsStates": map[string]any{
			"child": map[string]any{"status": "completed", "message": subagentMessage},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := []servicechat.Event{
		{Seq: 1, T: 1, Type: "user", TurnID: "turn-1", Text: "question"},
		{Seq: 2, T: 2, Type: "provider_event", TurnID: "turn-1", Data: json.RawMessage(`{"raw":"telemetry"}`)},
		{Seq: 3, T: 3, Type: "tool_use_start", TurnID: "turn-1", ID: "tool-1", Name: "Bash", Input: json.RawMessage(`{"command":"test"}`)},
		{Seq: 4, T: 4, Type: "collaboration", TurnID: "turn-1", ID: "child", Name: "spawn", Status: "inProgress", Data: firstCollaboration},
		{Seq: 5, T: 5, Type: "tool_use_end", TurnID: "turn-1", ID: "tool-1", Output: toolOutput},
		{Seq: 6, T: 6, Type: "collaboration", TurnID: "turn-1", ID: "child", Name: "spawn", Status: "completed", Data: finalCollaboration},
		{Seq: 7, T: 7, Type: "complete", TurnID: "turn-1"},
		{Seq: 8, T: 8, Type: "user", TurnID: "turn-2", Text: "next question"},
		{Seq: 9, T: 9, Type: "complete", TurnID: "turn-2"},
	}
	store := testStore(t)
	makeChat(t, store, "abcd")
	appendEvents(t, store, "abcd", events...)

	page, err := store.ReadTranscriptPage(
		context.Background(), "abcd", servicechat.TranscriptPageQuery{Limit: 10},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 2 {
		t.Fatalf("displayed page = %#v", page)
	}
	var displayed []servicechat.Event
	for _, turn := range page.Turns {
		displayed = append(displayed, turn.Events...)
	}
	var collaborations int
	var toolRef, childToolRef, childMessageRef string
	for _, event := range displayed {
		switch event.Type {
		case "provider_event":
			t.Fatal("provider telemetry leaked into transcript")
		case "tool_use_end":
			toolRef = event.OutputRef
			if !event.OutputTruncated || event.OutputBytes != int64(len(toolOutput)) || len(event.Output) >= len(toolOutput) {
				t.Fatalf("tool preview = %#v", event)
			}
		case "collaboration":
			collaborations++
			if event.Status != "completed" || event.Seq != 4 {
				t.Fatalf("collaboration did not retain position and latest state: %#v", event)
			}
			var data map[string]any
			if err := json.Unmarshal(event.Data, &data); err != nil {
				t.Fatal(err)
			}
			tool := data["tools"].([]any)[0].(map[string]any)
			childToolRef, _ = tool["outputRef"].(string)
			state := data["agentsStates"].(map[string]any)["child"].(map[string]any)
			childMessageRef, _ = state["messageRef"].(string)
		}
	}
	if collaborations != 1 || toolRef == "" || childToolRef == "" || childMessageRef == "" {
		t.Fatalf("transcript refs: collaborations=%d tool=%q childTool=%q childMessage=%q", collaborations, toolRef, childToolRef, childMessageRef)
	}
	for ref, want := range map[string]string{
		toolRef:         toolOutput,
		childToolRef:    subagentOutput,
		childMessageRef: subagentMessage,
	} {
		if got := readAllTranscriptContent(t, store, "abcd", ref, 4_001); got != want {
			t.Fatalf("full content %q has %d bytes, want %d", ref, len(got), len(want))
		}
	}
}

func TestTranscriptPagesInsideOneLargeTurnByBytes(t *testing.T) {
	events := []servicechat.Event{{Seq: 1, T: 1, Type: "user", TurnID: "turn-1", Text: "question"}}
	for seq := int64(2); seq <= 11; seq++ {
		events = append(events, servicechat.Event{
			Seq: seq, T: seq, Type: "assistant_text", TurnID: "turn-1",
			MessageID: "message-" + string(rune('a'+seq)), Text: strings.Repeat("x", 900),
		})
	}
	events = append(events, servicechat.Event{Seq: 12, T: 12, Type: "complete", TurnID: "turn-1"})
	store := testStore(t)
	makeChat(t, store, "abcd")
	appendEvents(t, store, "abcd", events...)

	before := int64(0)
	seen := make(map[int64]struct{})
	pages := 0
	for {
		page, err := store.ReadTranscriptPage(context.Background(), "abcd", servicechat.TranscriptPageQuery{
			Limit: 10, BeforeSeq: before, ByteLimit: 1_500,
		})
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, turn := range page.Turns {
			for _, event := range turn.Events {
				if _, duplicate := seen[event.Seq]; duplicate {
					t.Fatalf("duplicate sequence %d", event.Seq)
				}
				seen[event.Seq] = struct{}{}
			}
		}
		if !page.HasMore {
			break
		}
		if page.NextBefore <= 0 || page.NextBefore >= before && before > 0 {
			t.Fatalf("unsafe cursor: before=%d page=%#v", before, page)
		}
		before = page.NextBefore
	}
	if pages < 2 || len(seen) != len(events) {
		t.Fatalf("pages=%d sequences=%d want=%d", pages, len(seen), len(events))
	}
}

func TestTranscriptContentPreservesByteOffsetBoundaries(t *testing.T) {
	output := strings.Repeat("a🙂z", 6_000)
	store := testStore(t)
	makeChat(t, store, "abcd")
	appendEvents(t, store, "abcd", servicechat.Event{Type: "tool_use_end", TurnID: "turn", ID: "tool", Output: output})
	page, err := store.ReadTranscriptPage(context.Background(), "abcd", servicechat.TranscriptPageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	contentID := page.Turns[0].Events[0].OutputRef
	for _, test := range []struct {
		name      string
		after     int64
		limit     int
		content   string
		next      int64
		complete  bool
		wantError error
	}{
		{name: "minimum limit ends before rune", limit: 1, content: "a", next: 1},
		{name: "minimum limit fits rune", after: 1, limit: 1, content: "🙂", next: 5},
		{name: "offset inside rune advances", after: 2, limit: 4, content: "za", next: 7},
		{name: "default limit", content: output, complete: true},
		{name: "end offset", after: int64(len(output)), limit: 4, complete: true},
		{name: "past end", after: int64(len(output)) + 1, wantError: servicechat.ErrTranscriptContentNotFound},
		{name: "negative offset", after: -1, wantError: servicechat.ErrTranscriptContentNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			chunk, err := store.ReadTranscriptContent(context.Background(), "abcd", contentID, test.after, test.limit)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("content error = %v, want %v", err, test.wantError)
			}
			if test.wantError != nil {
				return
			}
			if chunk.ContentID != contentID || chunk.Content != test.content || chunk.NextAfter != test.next ||
				chunk.Complete != test.complete || chunk.TotalBytes != int64(len(output)) {
				t.Fatalf("content chunk = %#v", chunk)
			}
		})
	}
}

func TestTranscriptPagesTenThenTwentyTurnsWithoutGaps(t *testing.T) {
	events := make([]servicechat.Event, 31)
	for turn := 1; turn <= len(events); turn++ {
		events[turn-1] = servicechat.Event{
			Seq:    int64(turn),
			T:      int64(turn),
			Type:   "user",
			TurnID: fmt.Sprintf("turn-%d", turn),
			Text:   fmt.Sprintf("question-%d", turn),
		}
	}
	store := testStore(t)
	makeChat(t, store, "abcd")
	appendEvents(t, store, "abcd", events...)

	latest, err := store.ReadTranscriptPage(
		context.Background(),
		"abcd",
		servicechat.TranscriptPageQuery{Limit: 10},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertTurnPage(t, latest, 22, 31, 22, 31, true)

	older, err := store.ReadTranscriptPage(
		context.Background(),
		"abcd",
		servicechat.TranscriptPageQuery{Limit: 20, BeforeSeq: latest.NextBefore},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertTurnPage(t, older, 2, 21, 2, 31, true)

	oldest, err := store.ReadTranscriptPage(
		context.Background(),
		"abcd",
		servicechat.TranscriptPageQuery{Limit: 20, BeforeSeq: older.NextBefore},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertTurnPage(t, oldest, 1, 1, 0, 31, false)
}

func readAllTranscriptContent(
	t *testing.T,
	store *Store,
	chatID servicechat.ID,
	contentID string,
	limit int,
) string {
	t.Helper()
	var content strings.Builder
	var after int64
	for {
		page, err := store.ReadTranscriptContent(
			context.Background(), chatID, contentID, after, limit,
		)
		if err != nil {
			t.Fatal(err)
		}
		content.WriteString(page.Content)
		if page.Complete {
			return content.String()
		}
		if page.NextAfter <= after {
			t.Fatalf("content cursor did not advance: %#v", page)
		}
		after = page.NextAfter
	}
}

func assertTurnPage(
	t *testing.T,
	page servicechat.TranscriptPage,
	first int,
	last int,
	nextBefore int64,
	lastSeq int64,
	hasMore bool,
) {
	t.Helper()
	if len(page.Turns) != last-first+1 {
		t.Fatalf("turn count = %d, want %d: %#v", len(page.Turns), last-first+1, page)
	}
	if page.Turns[0].ID != fmt.Sprintf("turn-%d", first) ||
		page.Turns[len(page.Turns)-1].ID != fmt.Sprintf("turn-%d", last) ||
		page.NextBefore != nextBefore || page.LastSeq != lastSeq || page.HasMore != hasMore {
		t.Fatalf("turn page = %#v", page)
	}
}
