package sqlitechat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

var testContext = context.Background()

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}
func makeChat(t *testing.T, s *Store, id string) chat.Meta {
	t.Helper()
	m, err := s.Create(testContext, chat.Meta{ID: chat.ID(id), Title: id, Provider: chat.ProviderClaude, CreatedAt: 1, LastMessageAt: 1, LastReadAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func appendEvents(t *testing.T, s *Store, id string, events ...chat.Event) {
	t.Helper()
	for _, ev := range events {
		if _, err := s.AppendEvent(testContext, chat.ID(id), ev); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCRUDDurabilityAndExclusiveOwnership(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	makeChat(t, s, "abcd")
	if _, err := New(root); err == nil {
		t.Fatal("second writer acquired store")
	}
	appendEvents(t, s, "abcd", chat.Event{Type: "user", Text: "hello", T: 8}, chat.Event{Type: "assistant_text", Text: "world", T: 9})
	meta, err := s.Update(testContext, "abcd", func(m *chat.Meta) { m.Title = "changed"; m.SetSessionID(chat.ProviderClaude, "native") })
	if err != nil {
		t.Fatal(err)
	}
	if meta.LastMessageAt != 8 {
		t.Fatalf("meta %+v", meta)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	meta, err = s.Get(testContext, "abcd")
	if err != nil || meta.Title != "changed" || meta.SessionID(chat.ProviderClaude) != "native" {
		t.Fatalf("meta %+v err %v", meta, err)
	}
	events, err := s.ReadEvents(testContext, "abcd")
	if err != nil || len(events) != 2 || events[1].Seq != 2 {
		t.Fatalf("events %+v err %v", events, err)
	}
	var journal string
	var foreign, synchronous int
	if err = s.db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	_ = s.db.QueryRow("PRAGMA foreign_keys").Scan(&foreign)
	_ = s.db.QueryRow("PRAGMA synchronous").Scan(&synchronous)
	if journal != "wal" || foreign != 1 || synchronous != 2 {
		t.Fatalf("pragmas %s %d %d", journal, foreign, synchronous)
	}
	if err = s.Verify(testContext); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "chats")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("runtime created legacy chat files")
	}
}

func TestAppendAtomicOnProjectionFailure(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "abcd")
	_, err := s.db.Exec(`CREATE TRIGGER reject_projection BEFORE INSERT ON transcript_items BEGIN SELECT RAISE(ABORT,'projection rejected'); END;`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendEvent(testContext, "abcd", chat.Event{Type: "user", Text: "must roll back", T: 10}); err == nil {
		t.Fatal("expected failure")
	}
	events, err := s.ReadEvents(testContext, "abcd")
	if err != nil || len(events) != 0 {
		t.Fatalf("partial append %+v %v", events, err)
	}
	meta, _ := s.Get(testContext, "abcd")
	if meta.LastMessageAt != 1 {
		t.Fatalf("metadata committed without event %+v", meta)
	}
}

func TestConcurrentAppendAndPaging(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "abcd")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 12; j++ {
				if _, err := s.AppendEvent(testContext, "abcd", chat.Event{Type: "assistant_text", Text: "piece"}); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	page, err := s.ReadEventsPage(testContext, "abcd", chat.EventPageQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 10 || page.Events[0].Seq != 87 || page.LastSeq != 96 || !page.HasMore || page.NextBefore != 87 {
		t.Fatalf("page %+v", page)
	}
	after, err := s.ReadEventsAfter(testContext, "abcd", 94)
	if err != nil || len(after) != 2 || after[0].Seq != 95 {
		t.Fatalf("after %+v %v", after, err)
	}
	var count int
	if err = s.ScanEvents(testContext, "abcd", func(ev chat.Event) {
		count++
		if ev.Seq != int64(count) {
			t.Errorf("seq %d count %d", ev.Seq, count)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if count != 96 {
		t.Fatalf("count %d", count)
	}
}

func TestForkRewindRetainPrefixWithoutCopyAndDeleteSource(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	appendEvents(t, s, "aaaa", chat.Event{Type: "user", TurnID: "one", T: 1, Text: "first"}, chat.Event{Type: "assistant_text", TurnID: "one", T: 2, Text: "answer"}, chat.Event{Type: "user", TurnID: "two", T: 3, Text: "second"}, chat.Event{Type: "assistant_text", TurnID: "two", T: 4, Text: "future"})
	_, err := s.ForkChat(testContext, "aaaa", chat.Meta{ID: "bbbb", Provider: chat.ProviderClaude, Sessions: chat.SessionIDs{chat.ProviderClaude: "native"}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM events").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatal("fork copied events")
	}
	meta, _ := s.Get(testContext, "bbbb")
	if meta.SessionID(chat.ProviderClaude) != "" {
		t.Fatal("fork retained future native state")
	}
	appendEvents(t, s, "bbbb", chat.Event{Type: "user", TurnID: "fork", T: 5, Text: "forked"})
	if err = s.RewindBranch(testContext, "aaaa", 3); err != nil {
		t.Fatal(err)
	}
	appendEvents(t, s, "aaaa", chat.Event{Type: "user", TurnID: "rewound", T: 6, Text: "replacement"})
	a, _ := s.ReadEvents(testContext, "aaaa")
	b, _ := s.ReadEvents(testContext, "bbbb")
	if len(a) != 3 || a[2].Text != "replacement" || len(b) != 3 || b[2].Text != "forked" {
		t.Fatalf("forks a %+v b %+v", a, b)
	}
	page, err := s.ReadTranscriptPage(testContext, "bbbb", chat.TranscriptPageQuery{Limit: 1})
	if err != nil || len(page.Turns) != 1 || page.Turns[0].ID != "fork" || !page.HasMore {
		t.Fatalf("projection %+v %v", page, err)
	}
	if err = s.Delete(testContext, "aaaa"); err != nil {
		t.Fatal(err)
	}
	b, err = s.ReadEvents(testContext, "bbbb")
	if err != nil || len(b) != 3 {
		t.Fatalf("lost inherited prefix %+v %v", b, err)
	}
	if err = s.Delete(testContext, "bbbb"); err != nil {
		t.Fatal(err)
	}
	_ = s.db.QueryRow("SELECT count(*) FROM events").Scan(&count)
	if count != 0 {
		t.Fatal("unreferenced events retained")
	}
	if err = s.Verify(testContext); err != nil {
		t.Fatal(err)
	}
}

func TestVersionedSnapshotDoesNotLeakIntoFork(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	appendEvents(t, s, "aaaa", chat.Event{Type: "user", TurnID: "one", Text: "run"}, chat.Event{Type: "collaboration", TurnID: "one", ID: "agent", Data: json.RawMessage(`{"message":"old state"}`)})
	_, err := s.ForkChat(testContext, "aaaa", chat.Meta{ID: "bbbb"}, -1)
	if err != nil {
		t.Fatal(err)
	}
	appendEvents(t, s, "aaaa", chat.Event{Type: "collaboration", TurnID: "one", ID: "agent", Data: json.RawMessage(`{"message":"new state"}`)})
	a, err := s.ReadTranscriptPage(testContext, "aaaa", chat.TranscriptPageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.ReadTranscriptPage(testContext, "bbbb", chat.TranscriptPageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Turns[0].Events) != 2 || len(b.Turns[0].Events) != 2 || string(a.Turns[0].Events[1].Data) != `{"message":"new state"}` || string(b.Turns[0].Events[1].Data) != `{"message":"old state"}` {
		t.Fatalf("snapshot leak a %+v b %+v", a, b)
	}
	hits, err := s.SearchTranscript(testContext, "bbbb", "new state", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("search leaked %+v %v", hits, err)
	}
}

func TestLargeContentBoundedUTF8AndSearchBeyondPreview(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	makeChat(t, s, "bbbb")
	output := strings.Repeat("a🙂z", 20000) + " needleafterpreview"
	appendEvents(t, s, "aaaa", chat.Event{Type: "user", TurnID: "one", Text: "run"}, chat.Event{Type: "tool_use_start", TurnID: "one", ID: "tool", Input: json.RawMessage(`{"command":"cat log"}`)}, chat.Event{Type: "tool_use_end", TurnID: "one", ID: "tool", Output: output})
	page, err := s.ReadTranscriptPage(testContext, "aaaa", chat.TranscriptPageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	ev := page.Turns[0].Events[2]
	if ev.OutputRef == "" || !ev.OutputTruncated || len(ev.Output) > 32768 {
		t.Fatalf("not compact %+v", ev)
	}
	var assembled strings.Builder
	var after int64
	for {
		chunk, err := s.ReadTranscriptContent(testContext, "aaaa", ev.OutputRef, after, 4096)
		if err != nil {
			t.Fatal(err)
		}
		if len(chunk.Content) > 4096 {
			t.Fatal("unbounded content")
		}
		assembled.WriteString(chunk.Content)
		if chunk.Complete {
			break
		}
		if chunk.NextAfter <= after {
			t.Fatal("non advancing cursor")
		}
		after = chunk.NextAfter
	}
	if assembled.String() != output {
		t.Fatal("content changed")
	}
	chunk, err := s.ReadTranscriptContent(testContext, "aaaa", ev.OutputRef, 2, 4)
	if err != nil || chunk.Content != "za" || chunk.NextAfter != 7 {
		t.Fatalf("UTF8 chunk %+v %v", chunk, err)
	}
	if _, err = s.ReadTranscriptContent(testContext, "bbbb", ev.OutputRef, 0, 10); !errors.Is(err, chat.ErrTranscriptContentNotFound) {
		t.Fatalf("cross-chat content access %v", err)
	}
	hits, err := s.SearchTranscript(testContext, "aaaa", "needleafterpreview", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("search %+v %v", hits, err)
	}
	hits, err = s.SearchTranscript(testContext, "bbbb", "needleafterpreview", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("search cross-chat %+v %v", hits, err)
	}
	if err = s.Delete(testContext, "aaaa"); err != nil {
		t.Fatal(err)
	}
	var count int
	_ = s.db.QueryRow("SELECT count(*) FROM content_blobs").Scan(&count)
	if count != 0 {
		t.Fatal("unreferenced content retained")
	}
}

func TestAuthoritativeCorruptionNeverRecreated(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, Filename)
	bad := []byte("this is not sqlite")
	if err := os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := New(root); err == nil {
		s.Close()
		t.Fatal("corrupt database opened")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(bad) {
		t.Fatal("corrupt database was overwritten")
	}
}

func TestIndexedPlans(t *testing.T) {
	s := testStore(t)
	for _, query := range []string{`SELECT payload_json FROM events WHERE branch_id='x' AND seq>12 AND seq<=20 ORDER BY seq`, `SELECT payload_json FROM transcript_items WHERE branch_id='x' AND start_seq<200 ORDER BY start_seq DESC,end_seq DESC`} {
		rows, err := s.db.Query("EXPLAIN QUERY PLAN " + query)
		if err != nil {
			t.Fatal(err)
		}
		var detail strings.Builder
		for rows.Next() {
			var a, b, c int
			var d string
			if err = rows.Scan(&a, &b, &c, &d); err != nil {
				t.Fatal(err)
			}
			detail.WriteString(d)
		}
		rows.Close()
		if !strings.Contains(detail.String(), "SEARCH") || strings.Contains(detail.String(), "USE TEMP B-TREE") {
			t.Fatalf("unindexed plan: %s", detail.String())
		}
	}
}

func TestTextSearchAssemblesDeltasAndRetainsHistoricalCutoff(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	appendEvents(t, s, "aaaa", chat.Event{Type: "user", TurnID: "one", Text: "question"}, chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "m", Text: "consol"}, chat.Event{Type: "provider_event"}, chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "m", Text: "idated "}, chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "m", Text: "history"})
	hits, err := s.SearchTranscript(testContext, "aaaa", "consolidated history", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("cross-delta search %+v %v", hits, err)
	}
	_, err = s.ForkChat(testContext, "aaaa", chat.Meta{ID: "bbbb"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	hits, err = s.SearchTranscript(testContext, "bbbb", "consolidated", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("prefix phrase missing %+v %v", hits, err)
	}
	hits, err = s.SearchTranscript(testContext, "bbbb", "history", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("future leaked %+v %v", hits, err)
	}
	appendEvents(t, s, "aaaa", chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "m", Text: " futureanswer"}, chat.Event{Type: "complete", TurnID: "one"})
	hits, err = s.SearchTranscript(testContext, "bbbb", "futureanswer", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("new future leaked %+v %v", hits, err)
	}
	if err = s.Delete(testContext, "aaaa"); err != nil {
		t.Fatal(err)
	}
	hits, err = s.SearchTranscript(testContext, "bbbb", "consolidated", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("deleted source lost search %+v %v", hits, err)
	}
}

func TestTextSearchAcrossChunkBoundaryAndUTF8(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	text := strings.Repeat("a ", 4093) + "boundaryphrase 🙂 end"
	appendEvents(t, s, "aaaa", chat.Event{Type: "assistant_text", TurnID: "one", Text: text}, chat.Event{Type: "complete", TurnID: "one"})
	hits, err := s.SearchTranscript(testContext, "aaaa", "boundaryphrase", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("chunk boundary search %+v %v", hits, err)
	}
	if err = s.Verify(testContext); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteSessionAndInputBlob(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	run, err := s.PrepareRun(testContext, chat.RunPreparation{ChatID: "aaaa", RunID: "test-run", TurnID: "turn", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordRunInput(testContext, run.RunID, "input", "prompt_context"); err != nil {
		t.Fatal(err)
	}
	if err = s.AcceptRunInput(testContext, run.RunID, chat.InputAcceptance{NativeSessionID: "native", Evidence: "test"}); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(testContext, "aaaa"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"runs", "handoffs", "harness_sessions", "content_blobs"} {
		var count int
		if err = s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d", table, count)
		}
	}
	if err = s.Verify(testContext); err != nil {
		t.Fatal(err)
	}
}

func TestLongHistoryIndexedReadsAndIsolation(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	makeChat(t, s, "bbbb")
	branch, _, err := activeBranch(testContext, s.db, "aaaa")
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	for batch := 0; batch < 20; batch++ {
		tx, err := s.db.BeginTx(testContext, nil)
		if err != nil {
			t.Fatal(err)
		}
		for i := batch * 500; i < (batch+1)*500; i++ {
			turn := i / 100
			offset := i % 100
			event := chat.Event{Seq: int64(i + 1), T: int64(i + 1), TurnID: fmt.Sprintf("t%03d", turn), Type: "assistant_text", Text: fmt.Sprintf("piece%d ", offset), MessageID: fmt.Sprintf("m%d", turn)}
			if offset == 0 {
				event.Type = "user"
				event.Text = fmt.Sprintf("question %d", turn)
			}
			if offset == 99 {
				event.Type = "complete"
				event.Text = ""
			}
			if turn == 99 && offset == 97 {
				event.Text = "uniqueneedle "
			}
			if turn == 99 && offset == 98 {
				event.Text = "crossdelta"
			}
			if _, err = s.appendEventTx(testContext, tx, branch, event, nil); err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	importTime := time.Since(started)
	appendEvents(t, s, "bbbb", chat.Event{Type: "user", Text: "another project's confidential message"})
	started = time.Now()
	page, err := s.ReadTranscriptPage(testContext, "aaaa", chat.TranscriptPageQuery{BeforeSeq: 5001, Limit: 2})
	pageTime := time.Since(started)
	if err != nil || len(page.Turns) != 2 || page.Turns[0].ID != "t048" || page.Turns[1].ID != "t049" || page.LastSeq != 10000 {
		t.Fatalf("long page %+v %v", page, err)
	}
	started = time.Now()
	hits, err := s.SearchTranscript(testContext, "aaaa", "uniqueneedle crossdelta", 10)
	searchTime := time.Since(started)
	if err != nil || len(hits) != 1 {
		t.Fatalf("long search %+v %v", hits, err)
	}
	hits, err = s.SearchTranscript(testContext, "aaaa", "confidential", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("cross-chat search %+v %v", hits, err)
	}
	started = time.Now()
	_, err = s.ForkChat(testContext, "aaaa", chat.Meta{ID: "cccc"}, 5000)
	forkTime := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	_ = s.db.QueryRow("SELECT count(*) FROM events").Scan(&count)
	if count != 10001 {
		t.Fatalf("fork duplicated records %d", count)
	}
	hits, err = s.SearchTranscript(testContext, "cccc", "uniqueneedle", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("fork future search %+v %v", hits, err)
	}
	started = time.Now()
	err = s.RewindBranch(testContext, "aaaa", 5001)
	rewindTime := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	page, err = s.ReadTranscriptPage(testContext, "aaaa", chat.TranscriptPageQuery{Limit: 1})
	if err != nil || page.LastSeq != 5000 || len(page.Turns) != 1 || page.Turns[0].ID != "t049" {
		t.Fatalf("rewound page %+v %v", page, err)
	}
	t.Logf("10,000 events / 100 turns: batched import %s; indexed older page %s; cross-delta FTS %s; prefix fork %s; rewind %s", importTime, pageTime, searchTime, forkTime, rewindTime)
}
