package filechat

import (
	"context"
	"fmt"
	"testing"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// benchBackends and benchChatSizes are the cross product every benchmark runs
// over. The sizes are large enough to expose a per-event scan and small enough
// that seeding a sample stays inside a normal benchmark run.
var (
	benchBackends  = []Backend{BackendJSONL, BackendSQLite}
	benchChatSizes = []int{1_000, 10_000}
)

// benchTurnEvents is how many events share a turn, so a page of 10 turns reads
// a bounded slice of a much larger conversation.
const benchTurnEvents = 4

func benchEvent(seq int64) servicechat.Event {
	return servicechat.Event{
		T:      1_700_000_000_000 + seq,
		Type:   "assistant_text",
		TurnID: fmt.Sprintf("t%d", seq/benchTurnEvents),
		Text:   "a line of generated text that is about the size of a real one",
	}
}

// benchOutOfOrderEvent timestamps the last event below the rewind cutoff while
// the events before it are above it, so the survivors are not a prefix of the
// sequence and SQLite has to fall back to rewriting the whole stream.
func benchOutOfOrderEvent(seq int64, n int) servicechat.Event {
	ev := benchEvent(seq)
	switch {
	case seq <= int64(n/2):
		// kept, in order
	case seq < int64(n):
		// dropped: far above the cutoff
		ev.T = benchEvent(int64(n) * 10).T
	default:
		// kept, but it follows the dropped ones
		ev.T = benchEvent(1).T
	}
	return ev
}

// benchStore opens a store on a fresh data directory that is removed when the
// benchmark finishes.
func benchStore(tb testing.TB, backend Backend) *Store {
	tb.Helper()
	store, err := NewWithBackend(tb.TempDir(), backend)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = store.Close() })
	return store
}

func benchCreate(tb testing.TB, store *Store, id servicechat.ID) {
	tb.Helper()
	if _, err := store.Create(context.Background(), servicechat.Meta{
		ID:        id,
		CreatedAt: 1,
	}); err != nil {
		tb.Fatal(err)
	}
}

func benchSeed(
	tb testing.TB,
	store *Store,
	id servicechat.ID,
	n int,
	build func(seq int64, n int) servicechat.Event,
) {
	tb.Helper()
	ctx := context.Background()
	benchCreate(tb, store, id)
	for seq := 1; seq <= n; seq++ {
		if _, err := store.AppendEvent(ctx, id, build(int64(seq), n)); err != nil {
			tb.Fatal(err)
		}
	}
}

// benchSeedOrdered seeds a conversation whose timestamps advance with the
// sequence, the shape a real chat has.
func benchSeedOrdered(tb testing.TB, store *Store, id servicechat.ID, n int) {
	tb.Helper()
	benchSeed(tb, store, id, n, func(seq int64, _ int) servicechat.Event {
		return benchEvent(seq)
	})
}

// benchWarmTranscript reads pages until the projection reports complete, so a
// read benchmark measures the steady state and not the first backfill.
func benchWarmTranscript(tb testing.TB, store *Store, id servicechat.ID) {
	tb.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(30 * time.Second)
	for {
		page, err := store.ReadTranscriptPage(ctx, id, servicechat.TranscriptPageQuery{Limit: 10})
		if err != nil {
			tb.Fatal(err)
		}
		if page.Indexing == nil {
			return
		}
		if time.Now().After(deadline) {
			tb.Fatalf("transcript never finished projecting: %#v", page)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// BenchmarkAppend measures writing one more event onto an existing
// conversation, which is the path every prompt run takes.
func BenchmarkAppend(b *testing.B) {
	for _, backend := range benchBackends {
		for _, size := range benchChatSizes {
			b.Run(fmt.Sprintf("%s/existing=%d", backend, size), func(b *testing.B) {
				ctx := context.Background()
				store := benchStore(b, backend)
				id := servicechat.ID("bbbb")
				benchSeedOrdered(b, store, id, size)

				next := int64(size)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					next++
					if _, err := store.AppendEvent(ctx, id, benchEvent(next)); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkTranscriptPage measures the first page a browser requests when a
// chat opens, against a conversation much larger than the page.
func BenchmarkTranscriptPage(b *testing.B) {
	query := servicechat.TranscriptPageQuery{Limit: 10}
	for _, backend := range benchBackends {
		for _, size := range benchChatSizes {
			b.Run(fmt.Sprintf("%s/conversation=%d", backend, size), func(b *testing.B) {
				ctx := context.Background()
				store := benchStore(b, backend)
				id := servicechat.ID("bbbb")
				benchSeedOrdered(b, store, id, size)
				benchWarmTranscript(b, store, id)

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := store.ReadTranscriptPage(ctx, id, query); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkForkCopy measures copying a whole conversation onto a fresh chat,
// which is what Fork does.
func BenchmarkForkCopy(b *testing.B) {
	for _, backend := range benchBackends {
		for _, size := range benchChatSizes {
			b.Run(fmt.Sprintf("%s/conversation=%d", backend, size), func(b *testing.B) {
				ctx := context.Background()
				store := benchStore(b, backend)
				src := servicechat.ID("bbbb")
				dst := servicechat.ID("cccc")
				benchSeedOrdered(b, store, src, size)
				benchCreate(b, store, dst)

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					// Clearing the destination is setup, not the copy.
					b.StopTimer()
					benchCreate(b, store, dst)
					b.StartTimer()
					if _, err := store.CopyEventStream(ctx, src, dst); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkRewind measures dropping the second half of a conversation. Rewind
// destroys what it measures, so every iteration restores the source from a
// template outside the timer.
//
// The in-order variant has timestamps advancing with the sequence, so SQLite
// takes the prefix delete. The out-of-order variant keeps one survivor after a
// dropped event, which sends SQLite through the whole-stream rewrite the rewind
// used before it was batched. Same store, same event count, two paths: the pair
// is the before and after of that change.
func BenchmarkRewind(b *testing.B) {
	orders := []struct {
		name  string
		build func(seq int64, n int) servicechat.Event
	}{
		{name: "in-order", build: func(seq int64, _ int) servicechat.Event {
			return benchEvent(seq)
		}},
		{name: "out-of-order", build: benchOutOfOrderEvent},
	}
	for _, backend := range benchBackends {
		for _, size := range benchChatSizes {
			for _, order := range orders {
				b.Run(
					fmt.Sprintf("%s/%s/conversation=%d", backend, order.name, size),
					func(b *testing.B) {
						ctx := context.Background()
						store := benchStore(b, backend)
						template := servicechat.ID("aaaa")
						id := servicechat.ID("bbbb")
						benchSeed(b, store, template, size, order.build)
						benchCreate(b, store, id)

						// Keep the first half so the rewind has a real tail to drop.
						keepT := benchEvent(int64(size/2) + 1).T
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							b.StopTimer()
							benchCreate(b, store, id)
							if _, err := store.CopyEventStream(ctx, template, id); err != nil {
								b.Fatal(err)
							}
							b.StartTimer()
							if err := store.TruncateEventsBefore(ctx, id, keepT); err != nil {
								b.Fatal(err)
							}
						}
					},
				)
			}
		}
	}
}
