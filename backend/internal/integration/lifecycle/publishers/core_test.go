package publishers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type contextKey string

const testContextKey contextKey = "test"

type recordedCall struct {
	kind  string
	event UpdateEvent
	err   error
	ctx   context.Context
}

type recordingObserver struct {
	name  string
	calls *[]recordedCall
}

func (o recordingObserver) OnUpdateStarted(ctx context.Context, event UpdateEvent) {
	*o.calls = append(*o.calls, recordedCall{kind: o.name + ":started", event: event, ctx: ctx})
}

func (o recordingObserver) OnUpdateCompleted(ctx context.Context, event UpdateEvent) {
	*o.calls = append(*o.calls, recordedCall{kind: o.name + ":completed", event: event, ctx: ctx})
}

func (o recordingObserver) OnUpdateFailed(ctx context.Context, event UpdateEvent, err error) {
	*o.calls = append(*o.calls, recordedCall{kind: o.name + ":failed", event: event, err: err, ctx: ctx})
}

func TestSubscriberReceivesStartedCompletedAndFailed(t *testing.T) {
	c := New()
	var calls []recordedCall
	c.Subscribe(recordingObserver{name: "a", calls: &calls})

	ctx := context.Background()
	c.PublishUpdateStarted(ctx, "src", "op", "subj")
	c.PublishUpdateCompleted(ctx, "src", "op", "subj")
	c.PublishUpdateFailed(ctx, "src", "op", "subj", errors.New("boom"))

	if len(calls) != 3 {
		t.Fatalf("len(calls) = %d, want 3", len(calls))
	}
	if calls[0].kind != "a:started" || calls[1].kind != "a:completed" || calls[2].kind != "a:failed" {
		t.Fatalf("unexpected call sequence: %+v", calls)
	}
}

func TestMultipleSubscribersRunInRegistrationOrder(t *testing.T) {
	c := New()
	var calls []recordedCall
	c.Subscribe(recordingObserver{name: "first", calls: &calls})
	c.Subscribe(recordingObserver{name: "second", calls: &calls})
	c.Subscribe(recordingObserver{name: "third", calls: &calls})

	c.PublishUpdateStarted(context.Background(), "src", "op", "subj")

	if len(calls) != 3 {
		t.Fatalf("len(calls) = %d, want 3", len(calls))
	}
	want := []string{"first:started", "second:started", "third:started"}
	for i, w := range want {
		if calls[i].kind != w {
			t.Fatalf("calls[%d].kind = %q, want %q", i, calls[i].kind, w)
		}
	}
}

func TestUnsubscribedObserverReceivesNoLaterEvents(t *testing.T) {
	c := New()
	var calls []recordedCall
	unsubscribe := c.Subscribe(recordingObserver{name: "a", calls: &calls})

	c.PublishUpdateStarted(context.Background(), "src", "op", "subj")
	unsubscribe()
	c.PublishUpdateCompleted(context.Background(), "src", "op", "subj")

	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1 (only the pre-unsubscribe event)", len(calls))
	}

	// Calling unsubscribe again must be a harmless no-op.
	unsubscribe()
}

func TestPublishingWithNoObserversIsANoop(t *testing.T) {
	c := New()
	c.PublishUpdateStarted(context.Background(), "src", "op", "subj")
	c.PublishUpdateCompleted(context.Background(), "src", "op", "subj")
	c.PublishUpdateFailed(context.Background(), "src", "op", "subj", errors.New("boom"))
}

func TestExactContextEventFieldsAndErrorReachObservers(t *testing.T) {
	c := New()
	var calls []recordedCall
	c.Subscribe(recordingObserver{name: "a", calls: &calls})

	ctx := context.WithValue(context.Background(), testContextKey, "value")
	failErr := errors.New("specific failure")
	c.PublishUpdateFailed(ctx, "two-factor", "disable", "user@example.com", failErr)

	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1", len(calls))
	}
	got := calls[0]
	if got.ctx.Value(testContextKey) != "value" {
		t.Fatal("observer did not receive the exact supplied context")
	}
	want := UpdateEvent{Source: "two-factor", Operation: "disable", Subject: "user@example.com"}
	if got.event != want {
		t.Fatalf("event = %+v, want %+v", got.event, want)
	}
	if !errors.Is(got.err, failErr) {
		t.Fatalf("err = %v, want %v", got.err, failErr)
	}
}

// countingObserver only increments a counter under its own lock. Concurrent
// Publish calls may legitimately invoke one subscribed observer from many
// goroutines at once (Core makes no serialization promise about that); this
// double is written to tolerate it, so the test isolates whether Core's own
// registry (Subscribe/unsubscribe/dispatch) is race-free.
type countingObserver struct {
	mu    *sync.Mutex
	count *int
}

func (o countingObserver) OnUpdateStarted(context.Context, UpdateEvent) {
	o.mu.Lock()
	*o.count++
	o.mu.Unlock()
}
func (o countingObserver) OnUpdateCompleted(context.Context, UpdateEvent)     {}
func (o countingObserver) OnUpdateFailed(context.Context, UpdateEvent, error) {}

func TestSubscriptionAndPublicationAreSafeWhenConcurrent(t *testing.T) {
	c := New()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var mu sync.Mutex
			count := 0
			unsubscribe := c.Subscribe(countingObserver{mu: &mu, count: &count})
			defer unsubscribe()
			for j := 0; j < 20; j++ {
				c.PublishUpdateStarted(context.Background(), "src", "op", "subj")
			}
		}()
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				c.PublishUpdateCompleted(context.Background(), "src", "op", "subj")
				c.PublishUpdateFailed(context.Background(), "src", "op", "subj", errors.New("boom"))
			}
		}()
	}
	wg.Wait()
}

// reentrantObserver subscribes and unsubscribes another observer from
// inside its own callback, proving dispatch does not hold the registry
// mutex while invoking observers.
type reentrantObserver struct {
	core *Core
	done chan struct{}
}

func (o reentrantObserver) OnUpdateStarted(context.Context, UpdateEvent) {
	var calls []recordedCall
	unsubscribe := o.core.Subscribe(recordingObserver{name: "late", calls: &calls})
	unsubscribe()
	close(o.done)
}

func (o reentrantObserver) OnUpdateCompleted(context.Context, UpdateEvent)     {}
func (o reentrantObserver) OnUpdateFailed(context.Context, UpdateEvent, error) {}

func TestObserverMaySubscribeOrUnsubscribeFromWithinACallbackWithoutDeadlock(t *testing.T) {
	c := New()
	done := make(chan struct{})
	c.Subscribe(reentrantObserver{core: c, done: done})

	go c.PublishUpdateStarted(context.Background(), "src", "op", "subj")

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("observer subscribing/unsubscribing from within a callback deadlocked")
	}
}
