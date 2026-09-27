package filechat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func newSQLiteTestStore(t testing.TB, root string) *Store {
	t.Helper()
	store, err := NewWithBackend(root, BackendSQLite)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// waitForTranscript projects the stored stream until the page no longer
// reports index progress, which is how the projection converges after a read
// triggers a background backfill.
func waitForTranscript(t *testing.T, store *Store, id servicechat.ID) servicechat.TranscriptPage {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(5 * time.Second)
	for {
		page, err := store.ReadTranscriptPage(ctx, id, servicechat.TranscriptPageQuery{Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		if page.Indexing == nil {
			return page
		}
		if time.Now().After(deadline) {
			t.Fatalf("transcript never finished projecting: %#v", page)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// transcriptEventLines flattens a projected page so two backends can be
// compared turn by turn.
func transcriptEventLines(page servicechat.TranscriptPage) []string {
	lines := make([]string, 0, 8)
	for _, turn := range page.Turns {
		for _, event := range turn.Events {
			lines = append(lines, fmt.Sprintf(
				"%s|%d|%s|%s", turn.ID, event.Seq, event.Type, event.Text,
			))
		}
	}
	return lines
}

func transcriptFixture(toolOutput string) []servicechat.Event {
	return []servicechat.Event{
		{Seq: 1, T: 1, Type: "user", TurnID: "turn-1", Text: "question"},
		{Seq: 2, T: 2, Type: "provider_event", TurnID: "turn-1",
			Data: json.RawMessage(`{"raw":"telemetry"}`)},
		{Seq: 3, T: 3, Type: "tool_use_start", TurnID: "turn-1", ID: "tool-1",
			Name: "Bash", Input: json.RawMessage(`{"command":"test"}`)},
		{Seq: 4, T: 4, Type: "tool_use_end", TurnID: "turn-1", ID: "tool-1",
			Output: toolOutput},
		{Seq: 5, T: 5, Type: "complete", TurnID: "turn-1"},
		{Seq: 6, T: 6, Type: "user", TurnID: "turn-2", Text: "next question"},
		{Seq: 7, T: 7, Type: "complete", TurnID: "turn-2"},
	}
}

// TestSQLiteTranscriptProjectionMatchesJSONLProjection requires the folded
// projection inside chats.sqlite to present exactly what the standalone JSONL
// transcript index presents for the same archive.
func TestSQLiteTranscriptProjectionMatchesJSONLProjection(t *testing.T) {
	toolOutput := strings.Repeat("tool-result-🙂\n", 4_000)
	events := transcriptFixture(toolOutput)

	jsonlRoot, sqliteRoot := t.TempDir(), t.TempDir()
	writeStoredChat(t, jsonlRoot, "abcd", events)
	writeStoredChat(t, sqliteRoot, "abcd", events)
	jsonlStore := newIndexedTestStore(t, jsonlRoot)
	sqliteStore := newSQLiteTestStore(t, sqliteRoot)

	jsonlPage := waitForTranscript(t, jsonlStore, "abcd")
	sqlitePage := waitForTranscript(t, sqliteStore, "abcd")

	if len(sqlitePage.Turns) != 2 || len(jsonlPage.Turns) != 2 {
		t.Fatalf("turns: jsonl=%d sqlite=%d", len(jsonlPage.Turns), len(sqlitePage.Turns))
	}
	if sqlitePage.LastSeq != jsonlPage.LastSeq ||
		sqlitePage.NextBefore != jsonlPage.NextBefore ||
		sqlitePage.HasMore != jsonlPage.HasMore {
		t.Fatalf("page header: jsonl=%#v sqlite=%#v", jsonlPage, sqlitePage)
	}
	got := transcriptEventLines(sqlitePage)
	want := transcriptEventLines(jsonlPage)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("projected events differ:\njsonl: %v\nsqlite: %v", want, got)
	}
	for _, turn := range sqlitePage.Turns {
		for _, event := range turn.Events {
			if event.Type == "provider_event" {
				t.Fatalf("provider telemetry leaked into projection: %#v", event)
			}
		}
	}

	for ref := range transcriptRefs(t, sqliteStore, sqlitePage) {
		fromSQLite := readAllTranscriptContent(t, sqliteStore, "abcd", ref, 4_001)
		fromJSONL := readAllTranscriptContent(t, jsonlStore, "abcd", ref, 4_001)
		if fromSQLite != fromJSONL || !strings.Contains(fromSQLite, "tool-result") {
			t.Fatalf("content %q: sqlite=%d bytes jsonl=%d bytes",
				ref, len(fromSQLite), len(fromJSONL))
		}
	}
}

// transcriptRefs collects the collapsed content references of a projected
// page so each one can be read back in full.
func transcriptRefs(t *testing.T, store *Store, page servicechat.TranscriptPage) map[string]struct{} {
	t.Helper()
	refs := make(map[string]struct{})
	for _, turn := range page.Turns {
		for _, event := range turn.Events {
			if event.OutputRef != "" {
				refs[event.OutputRef] = struct{}{}
			}
		}
	}
	if len(refs) == 0 {
		t.Fatalf("projected page holds no collapsed content references: %#v", page)
	}
	return refs
}

// TestSQLiteTranscriptContentReadsSourceEventBySequence checks the collapsed
// content path: the reference stores a sequence, not a byte range.
func TestSQLiteTranscriptContentReadsSourceEventBySequence(t *testing.T) {
	toolOutput := strings.Repeat("tool-result-🙂\n", 4_000)
	root := t.TempDir()
	writeStoredChat(t, root, "abcd", transcriptFixture(toolOutput))
	store := newSQLiteTestStore(t, root)
	waitForTranscript(t, store, "abcd")

	var contentID, fieldKind string
	var sourceSeq, contentBytes int64
	err := store.sqlite.db.QueryRow(`
		SELECT content_id, field_kind, source_seq, content_bytes
		FROM chat_transcript_content_refs
		WHERE chat_id = ?`, "abcd",
	).Scan(&contentID, &fieldKind, &sourceSeq, &contentBytes)
	if err != nil {
		t.Fatal(err)
	}
	if fieldKind != "tool_output" || sourceSeq != 4 || contentBytes != int64(len(toolOutput)) {
		t.Fatalf("content ref kind=%q seq=%d bytes=%d", fieldKind, sourceSeq, contentBytes)
	}
	if got := readAllTranscriptContent(t, store, "abcd", contentID, 4_001); got != toolOutput {
		t.Fatalf("full content has %d bytes, want %d", len(got), len(toolOutput))
	}
	if store.index != nil {
		t.Fatal("sqlite mode must not open the standalone transcript index")
	}
	if _, err := os.Stat(filepath.Join(root, "transcript-index.sqlite")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("transcript-index.sqlite should not exist in sqlite mode: %v", err)
	}
}

// TestSQLiteTranscriptProgressCountsStoredPayloadBytes pins the O(1) progress
// bookkeeping to the stored stream, then checks it survives a rebuild.
func TestSQLiteTranscriptProgressCountsStoredPayloadBytes(t *testing.T) {
	root := t.TempDir()
	writeStoredChat(t, root, "abcd", transcriptFixture(strings.Repeat("x", 2_000)))
	store := newSQLiteTestStore(t, root)
	ctx := context.Background()
	waitForTranscript(t, store, "abcd")

	assertTrackedBytes := func(label string) int64 {
		t.Helper()
		var stored, tracked int64
		if err := store.sqlite.db.QueryRow(`
			SELECT COALESCE(SUM(LENGTH(payload)), 0) FROM chat_events WHERE chat_id = ?`,
			"abcd",
		).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if err := store.sqlite.db.QueryRow(
			`SELECT payload_bytes FROM chat_event_bytes WHERE chat_id = ?`,
			"abcd", &tracked,
		).Scan(&tracked); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if tracked != stored {
			t.Fatalf("%s: tracked %d payload bytes, want %d", label, tracked, stored)
		}
		return stored
	}

	totalBytes := assertTrackedBytes("after import")

	if _, err := store.AppendEvent(ctx, "abcd", servicechat.Event{
		T: 20, Type: "user", TurnID: "turn-3", Text: "appended",
	}); err != nil {
		t.Fatal(err)
	}
	assertTrackedBytes("after append")
	if err := store.TruncateEventsBefore(ctx, "abcd", 6); err != nil {
		t.Fatal(err)
	}
	totalBytes = assertTrackedBytes("after rewind")

	if err := store.deleteSQLiteProjection(ctx, "abcd"); err != nil {
		t.Fatal(err)
	}
	page, err := store.ReadTranscriptPage(
		ctx, "abcd", servicechat.TranscriptPageQuery{Limit: 50},
	)
	if err != nil {
		t.Fatal(err)
	}
	if page.Indexing == nil {
		t.Fatal("discarded projection did not report index progress")
	}
	if page.Indexing.TotalBytes != totalBytes || page.Indexing.IndexedBytes != 0 {
		t.Fatalf("progress = %#v, want %d/%d", page.Indexing, 0, totalBytes)
	}
	if !page.Indexing.TailSeqKnown {
		t.Fatal("imported archive should report a known tail sequence")
	}

	final := waitForTranscript(t, store, "abcd")
	if final.LastSeq != 5 || len(final.Turns) != 1 || final.Turns[0].ID != "turn-1" {
		t.Fatalf("rebuilt projection after rewind = %#v", final)
	}
	for _, turn := range final.Turns {
		if turn.ID == "turn-2" {
			t.Fatalf("rewound turn survived the rebuild: %#v", final)
		}
	}
}

// TestSQLiteAppendKeepsTranscriptProjectionReady covers the inline refresh
// after an append, so a reopened chat never shows index progress for work
// that has already been done.
func TestSQLiteAppendKeepsTranscriptProjectionReady(t *testing.T) {
	root := t.TempDir()
	writeStoredChat(t, root, "abcd", []servicechat.Event{
		{Seq: 1, T: 1, Type: "user", TurnID: "turn-1", Text: "question"},
		{Seq: 2, T: 2, Type: "complete", TurnID: "turn-1"},
	})
	store := newSQLiteTestStore(t, root)
	ctx := context.Background()
	waitForTranscript(t, store, "abcd")

	if _, err := store.AppendEvent(ctx, "abcd", servicechat.Event{
		T: 10, Type: "user", TurnID: "turn-2", Text: "follow up",
	}); err != nil {
		t.Fatal(err)
	}
	page, err := store.ReadTranscriptPage(
		ctx, "abcd", servicechat.TranscriptPageQuery{Limit: 50},
	)
	if err != nil {
		t.Fatal(err)
	}
	if page.Indexing != nil || len(page.Turns) != 2 {
		t.Fatalf("page after append = %#v", page)
	}
	if page.LastSeq != 3 {
		t.Fatalf("last sequence = %d, want 3", page.LastSeq)
	}
}

// TestSQLiteTranscriptEventWindowReadsTurnsBySequence checks the window
// fallback reads the projected turns by sequence instead of byte ranges.
func TestSQLiteTranscriptEventWindowReadsTurnsBySequence(t *testing.T) {
	root := t.TempDir()
	writeStoredChat(t, root, "abcd", transcriptFixture(strings.Repeat("x", 2_000)))
	store := newSQLiteTestStore(t, root)
	ctx := context.Background()
	page := waitForTranscript(t, store, "abcd")

	window, err := store.ReadTranscriptEventWindow(ctx, "abcd", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if window.LastSeq != page.LastSeq || window.LastSeq != 7 {
		t.Fatalf("window tail = %d, want %d", window.LastSeq, page.LastSeq)
	}
	if len(window.Events) == 0 || window.Events[len(window.Events)-1].Seq != 7 {
		t.Fatalf("window events = %#v", window.Events)
	}

	older, err := store.ReadTranscriptEventWindow(ctx, "abcd", 6, 1)
	if err != nil {
		t.Fatal(err)
	}
	// LastSeq is the tail of the whole chat, exactly as the JSONL window
	// reports it; the events themselves stop at the requested turn.
	if older.LastSeq != 7 || len(older.Events) == 0 ||
		older.Events[len(older.Events)-1].Seq != 5 {
		t.Fatalf("older window = %#v", older)
	}
}
