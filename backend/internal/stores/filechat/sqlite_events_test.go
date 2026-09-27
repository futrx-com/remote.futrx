package filechat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// eventLines renders events as comparable lines so two backends can be diffed
// step by step.
func eventLines(events []servicechat.Event) []string {
	lines := make([]string, 0, len(events))
	for _, event := range events {
		lines = append(lines, fmt.Sprintf("%d|%s|%s", event.Seq, event.Type, event.Text))
	}
	return lines
}

func pageLine(page servicechat.EventPage) string {
	return fmt.Sprintf("events=%s hasMore=%v next=%d last=%d",
		strings.Join(eventLines(page.Events), ","), page.HasMore, page.NextBefore, page.LastSeq)
}

// TestEventBackendsMatchAcrossTheChatLifecycle runs one scripted chat against
// both storage engines and requires every observation to agree, so the SQLite
// backend cannot quietly change the contracts callers already rely on.
func TestEventBackendsMatchAcrossTheChatLifecycle(t *testing.T) {
	fromJSONL := runEventBackendScenario(t, BackendJSONL)
	fromSQLite := runEventBackendScenario(t, BackendSQLite)

	if len(fromJSONL) != len(fromSQLite) {
		t.Fatalf("observations = %d, want %d", len(fromSQLite), len(fromJSONL))
	}
	for i := range fromJSONL {
		if fromJSONL[i] != fromSQLite[i] {
			t.Fatalf("step %d: jsonl %q, sqlite %q", i, fromJSONL[i], fromSQLite[i])
		}
	}
	if len(fromJSONL) == 0 {
		t.Fatal("scenario recorded no observations")
	}
}

func runEventBackendScenario(t *testing.T, backend Backend) []string {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	store, err := NewWithBackend(root, backend)
	if err != nil {
		t.Fatal(err)
	}

	var seen []string
	note := func(label string, events []servicechat.Event, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		seen = append(seen, label+":"+strings.Join(eventLines(events), ","))
	}

	if _, err := store.Create(ctx, servicechat.Meta{ID: "abcd", CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	appends := []servicechat.Event{
		{T: 10, Type: "user", TurnID: "t1", Text: "question"},
		{T: 11, Type: "assistant_text", TurnID: "t1", MessageID: "m1", Text: "answer"},
		{T: 12, Type: "tool", TurnID: "t1", Name: "bash"},
		{T: 13, Type: "user", TurnID: "t2", Text: "second"},
		{T: 14, Type: "complete", TurnID: "t2", Text: "done"},
	}
	for i, event := range appends {
		stored, err := store.AppendEvent(ctx, "abcd", event)
		note(fmt.Sprintf("append-%d", i), []servicechat.Event{stored}, err)
	}

	events, err := store.ReadEvents(ctx, "abcd")
	note("read", events, err)

	page, err := store.ReadEventsPage(ctx, "abcd", servicechat.EventPageQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	seen = append(seen, "page:"+pageLine(page))

	older, err := store.ReadEventsPage(ctx, "abcd", servicechat.EventPageQuery{
		Limit:     2,
		BeforeSeq: page.NextBefore,
	})
	if err != nil {
		t.Fatal(err)
	}
	seen = append(seen, "page-older:"+pageLine(older))

	after, err := store.ReadEventsAfter(ctx, "abcd", 2)
	note("after-2", after, err)

	var scanned []servicechat.Event
	if err := store.ScanEvents(ctx, "abcd", func(event servicechat.Event) {
		scanned = append(scanned, event)
	}); err != nil {
		t.Fatal(err)
	}
	note("scan", scanned, nil)

	kept, err := store.TruncateEventsBefore(ctx, "abcd", 13)
	note("rewind-kept", kept, err)
	events, err = store.ReadEvents(ctx, "abcd")
	note("read-after-rewind", events, err)

	stored, err := store.AppendEvent(ctx, "abcd", servicechat.Event{
		T: 15, Type: "user", Text: "after rewind",
	})
	note("append-after-rewind", []servicechat.Event{stored}, err)

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewWithBackend(root, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	events, err = reopened.ReadEvents(ctx, "abcd")
	note("reopen-read", events, err)
	return seen
}

// TestSQLiteBackendImportsExistingJSONLArchive covers the first read of a chat
// that was written before the SQLite backend existed.
func TestSQLiteBackendImportsExistingJSONLArchive(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	seedArchive(t, root, []servicechat.Event{
		{T: 10, Type: "user", TurnID: "t1", Text: "first"},
		{T: 11, Type: "assistant_text", TurnID: "t1", Text: "reply"},
		{T: 12, Type: "user", TurnID: "t2", Text: "second"},
	})

	store, err := NewWithBackend(root, BackendSQLite)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	events, err := store.ReadEvents(ctx, "abcd")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1|user|first", "2|assistant_text|reply", "3|user|second"}
	if got := eventLines(events); !slices.Equal(got, want) {
		t.Fatalf("imported events = %v, want %v", got, want)
	}

	stored, err := store.AppendEvent(ctx, "abcd", servicechat.Event{
		T: 13, Type: "complete", Text: "new",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Seq != 4 {
		t.Fatalf("seq after import = %d, want 4", stored.Seq)
	}

	mirror, err := os.ReadFile(filepath.Join(root, "chats", "abcd", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := nonEmptyLines(mirror); len(lines) != 4 {
		t.Fatalf("mirror lines = %d, want 4", len(lines))
	}
}

// TestSQLiteBackendRebuildsWhenArchiveIsRewritten covers the rollback path:
// the process ran on the JSONL backend long enough to rewrite an archive, and
// the database has to pick up the rewritten stream on its next read.
func TestSQLiteBackendRebuildsWhenArchiveIsRewritten(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	seedArchive(t, root, []servicechat.Event{
		{T: 10, Type: "user", Text: "old one"},
		{T: 11, Type: "user", Text: "old two"},
		{T: 12, Type: "user", Text: "old three"},
	})

	store, err := NewWithBackend(root, BackendSQLite)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	events, err := store.ReadEvents(ctx, "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("first read = %d events, want 3", len(events))
	}

	// Rewrite the archive the way a rollback on the JSONL backend would, then
	// append a line it never recorded.
	archive := filepath.Join(root, "chats", "abcd", "events.jsonl")
	rewritten := `{"seq":1,"t":20,"type":"user","text":"kept"}
{"seq":2,"t":21,"type":"user","text":"kept too"}
{"seq":3,"t":22,"type":"user","text":"tail"}
`
	if err := os.WriteFile(archive, []byte(rewritten), 0o644); err != nil {
		t.Fatal(err)
	}

	events, err = store.ReadEvents(ctx, "abcd")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1|user|kept", "2|user|kept too", "3|user|tail"}
	if got := eventLines(events); !slices.Equal(got, want) {
		t.Fatalf("events after rewrite = %v, want %v", got, want)
	}
}

// TestSQLiteBackendAppendsWhenTheArchiveIsGone proves the database stays
// writable when the rollback copy cannot be written or even found: the archive
// is a convenience, not a dependency.
func TestSQLiteBackendAppendsWhenTheArchiveIsGone(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := NewWithBackend(root, BackendSQLite)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	if _, err := store.Create(ctx, servicechat.Meta{ID: "abcd", CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(ctx, "abcd", servicechat.Event{
		T: 10, Type: "user", Text: "before",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "chats", "abcd")); err != nil {
		t.Fatal(err)
	}

	stored, err := store.AppendEvent(ctx, "abcd", servicechat.Event{
		T: 11, Type: "user", Text: "after",
	})
	if err != nil {
		t.Fatalf("append without an archive: %v", err)
	}
	if stored.Seq != 2 {
		t.Fatalf("seq = %d, want 2", stored.Seq)
	}

	events, err := store.ReadEvents(ctx, "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
}

func seedArchive(t *testing.T, root string, events []servicechat.Event) {
	t.Helper()
	store, err := NewWithBackend(root, BackendJSONL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), servicechat.Meta{ID: "abcd", CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if _, err := store.AppendEvent(context.Background(), "abcd", event); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func nonEmptyLines(data []byte) []string {
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
