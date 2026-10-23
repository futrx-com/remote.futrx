package filechat

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// TestCopyEventStreamMatchesSequentialAppend copies a conversation longer
// than one batch onto an empty chat and requires the destination to look
// exactly like an event-by-event append would have made it, for both backends.
func TestCopyEventStreamMatchesSequentialAppend(t *testing.T) {
	for _, backend := range []Backend{BackendJSONL, BackendSQLite} {
		t.Run(string(backend), func(t *testing.T) {
			root := t.TempDir()
			ctx := context.Background()
			store, err := NewWithBackend(root, backend)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })

			if _, err := store.Create(ctx, servicechat.Meta{ID: "abcd", CreatedAt: 1}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Create(ctx, servicechat.Meta{ID: "beef", CreatedAt: 1}); err != nil {
				t.Fatal(err)
			}

			// Longer than one copy batch, so the streaming path crosses a
			// transaction boundary.
			const events = copyBatchEvents + 200
			for i := 0; i < events; i++ {
				if _, err := store.AppendEvent(ctx, "abcd", servicechat.Event{
					T:      int64(i + 1),
					Type:   "user",
					TurnID: fmt.Sprintf("turn-%d", i),
					Text:   fmt.Sprintf("event %d", i),
				}); err != nil {
					t.Fatal(err)
				}
			}

			copied, err := store.CopyEventStream(ctx, "abcd", "beef")
			if err != nil {
				t.Fatal(err)
			}
			if copied != events {
				t.Fatalf("copied %d events, want %d", copied, events)
			}

			got, err := store.ReadEvents(ctx, "beef")
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != events {
				t.Fatalf("destination holds %d events, want %d", len(got), events)
			}
			for index, event := range got {
				if event.Seq != int64(index+1) {
					t.Fatalf("event %d has sequence %d", index, event.Seq)
				}
				if event.Type != "user" || event.Text != fmt.Sprintf("event %d", index) {
					t.Fatalf("event %d = %#v", index, event)
				}
			}

			meta, err := store.Get(ctx, "beef")
			if err != nil {
				t.Fatal(err)
			}
			if meta.LastMessageAt != int64(events) {
				t.Fatalf("destination last message at %d, want %d",
					meta.LastMessageAt, events)
			}

			page := waitCopiedTranscript(t, store, "beef")
			if page.LastSeq != int64(events) {
				t.Fatalf("destination projection tail = %d, want %d",
					page.LastSeq, events)
			}
			if got := countProjectedEvents(t, store, "beef"); got != events {
				t.Fatalf("projection holds %d events, want %d", got, events)
			}

			if backend == BackendSQLite {
				assertMirroredArchive(t, root, "beef", events)
			}
		})
	}
}

// TestCopyEventStreamOfAnEmptySource covers the degenerate copy a fork of an
// empty conversation performs, and the refusal to copy a chat onto itself.
func TestCopyEventStreamOfAnEmptySource(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	store, err := NewWithBackend(root, BackendSQLite)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, id := range []servicechat.ID{"abcd", "beef"} {
		if _, err := store.Create(ctx, servicechat.Meta{ID: id, CreatedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	copied, err := store.CopyEventStream(ctx, "abcd", "beef")
	if err != nil {
		t.Fatal(err)
	}
	if copied != 0 {
		t.Fatalf("copied %d events from an empty chat", copied)
	}
	if copied, err := store.CopyEventStream(ctx, "abcd", "abcd"); err != nil || copied != 0 {
		t.Fatalf("self copy = %d, %v", copied, err)
	}
	events, err := store.ReadEvents(ctx, "beef")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("destination holds %d events", len(events))
	}
}

// waitCopiedTranscript reads until the copied chat stops reporting progress.
func waitCopiedTranscript(t *testing.T, store *Store, id servicechat.ID) servicechat.TranscriptPage {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(5 * time.Second)
	for {
		page, err := store.ReadTranscriptPage(
			ctx, id, servicechat.TranscriptPageQuery{Limit: 100},
		)
		if err != nil {
			t.Fatal(err)
		}
		if page.Indexing == nil {
			return page
		}
		if time.Now().After(deadline) {
			t.Fatalf("copied transcript never finished projecting: %#v", page)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// countProjectedEvents walks every page of a chat's projection so a test can
// assert that the whole copied conversation materialized, not just the first
// page the service is willing to return.
func countProjectedEvents(t *testing.T, store *Store, id servicechat.ID) int {
	t.Helper()
	ctx := context.Background()
	total := 0
	var before int64
	for {
		page, err := store.ReadTranscriptPage(ctx, id, servicechat.TranscriptPageQuery{
			Limit:     100,
			BeforeSeq: before,
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, turn := range page.Turns {
			total += len(turn.Events)
		}
		if !page.HasMore {
			return total
		}
		if page.NextBefore <= 0 || (before > 0 && page.NextBefore >= before) {
			t.Fatalf("unsafe cursor: before=%d page=%#v", before, page)
		}
		before = page.NextBefore
	}
}

func assertMirroredArchive(t *testing.T, root string, id servicechat.ID, want int) {
	t.Helper()
	file, err := os.Open(filepath.Join(root, "chats", string(id), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxEventRecordBytes)
	lines := 0
	for scanner.Scan() {
		if len(scanner.Bytes()) > 0 {
			lines++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if lines != want {
		t.Fatalf("rollback archive holds %d records, want %d", lines, want)
	}
}
