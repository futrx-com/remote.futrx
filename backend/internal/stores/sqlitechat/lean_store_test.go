package sqlitechat

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func TestMissingSearchCacheRebuildsAfterRestart(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	makeChat(t, s, "aaaa")
	appendEvents(t, s, "aaaa", chat.Event{Type: "user", Text: "recoverableword"})
	if _, err = s.SearchTranscript(testContext, "aaaa", "recoverableword", 10); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("DROP TABLE transcript_search"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := s.SearchTranscript(testContext, "aaaa", "recoverableword", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("cache was not rebuilt from saved history: %d hits, %v", len(hits), err)
	}
	if _, err = s.db.Exec("DROP TABLE events"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if broken, err := New(root); err == nil {
		broken.Close()
		t.Fatal("missing authoritative history was silently recreated")
	}
}

func TestInvalidEventDoesNotAdvanceHistory(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	if _, err := s.AppendEvent(testContext, "aaaa", chat.Event{Type: "tool_use_start", Input: json.RawMessage(`{"broken":`)}); err == nil {
		t.Fatal("invalid JSON was accepted")
	}
	event, err := s.AppendEvent(testContext, "aaaa", chat.Event{Type: "user", Text: "valid"})
	if err != nil || event.Seq != 1 {
		t.Fatalf("failed append advanced the sequence: %d, %v", event.Seq, err)
	}
}

func TestConcurrentConversationsReadWhileStreaming(t *testing.T) {
	s := testStore(t)
	const conversations = 8
	const messages = 40
	for i := 0; i < conversations; i++ {
		makeChat(t, s, fmt.Sprintf("a%03d", i))
	}
	var workers sync.WaitGroup
	for i := 0; i < conversations; i++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			id := chat.ID(fmt.Sprintf("a%03d", index))
			for j := 0; j < messages; j++ {
				_, err := s.AppendEvent(testContext, id, chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "message", Text: "stream "})
				if err != nil {
					t.Error(err)
					return
				}
				if j%10 == 0 {
					page, err := s.ReadTranscriptPage(testContext, id, chat.TranscriptPageQuery{Limit: 1})
					if err != nil || page.LastSeq < int64(j+1) {
						t.Errorf("streaming page: seq %d, %v", page.LastSeq, err)
						return
					}
				}
			}
		}(i)
	}
	workers.Wait()
	for i := 0; i < conversations; i++ {
		page, err := s.ReadEventsPage(testContext, chat.ID(fmt.Sprintf("a%03d", i)), chat.EventPageQuery{Limit: 100})
		if err != nil || len(page.Events) != messages || page.LastSeq != messages {
			t.Fatalf("conversation %d: %d events, seq %d, %v", i, len(page.Events), page.LastSeq, err)
		}
	}
}
