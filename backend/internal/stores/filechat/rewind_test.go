package filechat

import (
	"context"
	"slices"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// TestRewindDropsExactlyTheFilteredEventsWhenTimestampsAreOutOfOrder proves the
// rewind keeps what the timestamp filter keeps even when the survivors are not
// a prefix of the sequence. SQLite detects that and falls back to rewriting the
// whole stream, so both backends must report the same events.
func TestRewindDropsExactlyTheFilteredEventsWhenTimestampsAreOutOfOrder(t *testing.T) {
	for _, backend := range []Backend{BackendJSONL, BackendSQLite} {
		t.Run(string(backend), func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			store, err := NewWithBackend(root, backend)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()

			if _, err := store.Create(ctx, servicechat.Meta{ID: "abcd", CreatedAt: 1}); err != nil {
				t.Fatal(err)
			}
			for _, event := range []servicechat.Event{
				{T: 10, Type: "user", Text: "kept"},
				{T: 30, Type: "assistant_text", Text: "dropped"},
				{T: 20, Type: "user", Text: "kept too"},
			} {
				if _, err := store.AppendEvent(ctx, "abcd", event); err != nil {
					t.Fatal(err)
				}
			}

			if err := store.TruncateEventsBefore(ctx, "abcd", 25); err != nil {
				t.Fatal(err)
			}

			events, err := store.ReadEvents(ctx, "abcd")
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"1|user|kept", "3|user|kept too"}
			if got := eventLines(events); !slices.Equal(got, want) {
				t.Fatalf("survivors = %v, want %v", got, want)
			}
			if backend == BackendSQLite {
				assertMirrorMatchesEvents(t, store, "abcd", want)
			}
		})
	}
}

// TestRewindKeepsTheSequenceOfASurvivingPrefix covers the common case where
// timestamps advance with the sequence: SQLite drops the tail with a single
// delete instead of rewriting the kept rows, and the mirror is rebuilt from
// the rows that survived.
func TestRewindKeepsTheSequenceOfASurvivingPrefix(t *testing.T) {
	for _, backend := range []Backend{BackendJSONL, BackendSQLite} {
		t.Run(string(backend), func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			store, err := NewWithBackend(root, backend)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()

			if _, err := store.Create(ctx, servicechat.Meta{ID: "abcd", CreatedAt: 1}); err != nil {
				t.Fatal(err)
			}
			for i := 1; i <= 5; i++ {
				if _, err := store.AppendEvent(ctx, "abcd", servicechat.Event{
					T:    int64(10 * i),
					Type: "user",
					Text: "event",
				}); err != nil {
					t.Fatal(err)
				}
			}

			if err := store.TruncateEventsBefore(ctx, "abcd", 31); err != nil {
				t.Fatal(err)
			}

			events, err := store.ReadEvents(ctx, "abcd")
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"1|user|event", "2|user|event", "3|user|event"}
			if got := eventLines(events); !slices.Equal(got, want) {
				t.Fatalf("survivors = %v, want %v", got, want)
			}

			meta, err := store.Get(ctx, "abcd")
			if err != nil {
				t.Fatal(err)
			}
			if meta.LastMessageAt != 30 {
				t.Fatalf("lastMessageAt = %d, want 30", meta.LastMessageAt)
			}
			if backend == BackendSQLite {
				assertMirrorMatchesEvents(t, store, "abcd", want)
			}
		})
	}
}

// TestRewindEverythingLeavesAnEmptyStreamWithAReusableSequence covers the edge
// the batched path shares with the old one: rewinding past the first event
// empties the stream, and the next append starts the sequence over at 1 in
// both storage and the mirror.
func TestRewindEverythingLeavesAnEmptyStreamWithAReusableSequence(t *testing.T) {
	for _, backend := range []Backend{BackendJSONL, BackendSQLite} {
		t.Run(string(backend), func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			store, err := NewWithBackend(root, backend)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()

			if _, err := store.Create(ctx, servicechat.Meta{ID: "abcd", CreatedAt: 1}); err != nil {
				t.Fatal(err)
			}
			for i := 1; i <= 3; i++ {
				if _, err := store.AppendEvent(ctx, "abcd", servicechat.Event{
					T:    int64(10 * i),
					Type: "user",
					Text: "event",
				}); err != nil {
					t.Fatal(err)
				}
			}

			if err := store.TruncateEventsBefore(ctx, "abcd", 1); err != nil {
				t.Fatal(err)
			}
			events, err := store.ReadEvents(ctx, "abcd")
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 0 {
				t.Fatalf("survivors = %v, want none", eventLines(events))
			}

			stored, err := store.AppendEvent(ctx, "abcd", servicechat.Event{
				T: 5, Type: "user", Text: "after",
			})
			if err != nil {
				t.Fatal(err)
			}
			if stored.Seq != 1 {
				t.Fatalf("seq after a full rewind = %d, want 1", stored.Seq)
			}
			if backend == BackendSQLite {
				assertMirrorMatchesEvents(t, store, "abcd", []string{"1|user|after"})
			}
		})
	}
}

// assertMirrorMatchesEvents reads events.jsonl directly, bypassing the backend
// selection, so it checks the rollback copy the SQLite engine writes.
func assertMirrorMatchesEvents(
	t *testing.T,
	store *Store,
	id servicechat.ID,
	want []string,
) {
	t.Helper()
	mirrored, err := store.readEventsFile(id)
	if err != nil {
		t.Fatalf("read mirror: %v", err)
	}
	if got := eventLines(mirrored); !slices.Equal(got, want) {
		t.Fatalf("mirror = %v, want %v", got, want)
	}
}
