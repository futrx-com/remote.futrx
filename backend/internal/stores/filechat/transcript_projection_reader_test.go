package filechat

import (
	"context"
	"errors"
	"strings"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func TestTranscriptContentPreservesByteOffsetBoundaries(t *testing.T) {
	root := t.TempDir()
	output := strings.Repeat("a🙂z", 6_000)
	writeStoredChat(t, root, "abcd", []servicechat.Event{
		{Seq: 1, Type: "tool_use_end", TurnID: "turn", ID: "tool", Output: output},
	})
	store := newIndexedTestStore(t, root)
	if _, err := store.index.syncChat(context.Background(), "abcd", store.eventsPath("abcd")); err != nil {
		t.Fatal(err)
	}
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

func TestTranscriptPageDecodesOnlyItemsInsideBudget(t *testing.T) {
	root := t.TempDir()
	writeStoredChat(t, root, "abcd", []servicechat.Event{
		{Seq: 1, Type: "user", TurnID: "older", Text: "older prompt"},
		{Seq: 2, Type: "user", TurnID: "newer", Text: "newer prompt"},
	})
	store := newIndexedTestStore(t, root)
	if _, err := store.index.syncChat(context.Background(), "abcd", store.eventsPath("abcd")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.index.db.Exec(`UPDATE chat_transcript_items SET payload_json = ? WHERE chat_id = ? AND start_seq = ?`,
		[]byte("invalid JSON"), "abcd", 1); err != nil {
		t.Fatal(err)
	}
	for _, query := range []servicechat.TranscriptPageQuery{
		{Limit: 1},
		{Limit: 2, ByteLimit: 1},
	} {
		page, err := store.ReadTranscriptPage(context.Background(), "abcd", query)
		if err != nil {
			t.Fatalf("query %#v: %v", query, err)
		}
		if len(page.Turns) != 1 || page.Turns[0].ID != "newer" || len(page.Turns[0].Events) != 1 ||
			page.Turns[0].Events[0].Text != "newer prompt" || !page.HasMore || page.NextBefore != 2 || page.LastSeq != 2 {
			t.Fatalf("query %#v: page = %#v", query, page)
		}
	}
	if _, err := store.ReadTranscriptPage(context.Background(), "abcd", servicechat.TranscriptPageQuery{Limit: 2}); !errors.Is(err, errInvalidChatEventIndex) {
		t.Fatalf("included invalid payload error = %v", err)
	}
}
