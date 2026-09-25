package lifecycle

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type eventBusContextKey struct{}

func TestEventBusQueuesWithoutWaitingAndDispatchesInOrder(t *testing.T) {
	parent := context.WithValue(context.Background(), eventBusContextKey{}, "bus")
	bus := newEventBus(parent, 4)
	t.Cleanup(bus.Close)

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	order := make(chan string, 4)
	var once sync.Once
	bus.Subscribe(func(ctx context.Context, event applications.Event) {
		if got := ctx.Value(eventBusContextKey{}); got != "bus" {
			t.Errorf("subscriber context value = %v, want bus lifecycle context", got)
		}
		if event.Name == "first" {
			once.Do(func() { close(firstEntered) })
			<-releaseFirst
		}
		order <- "one:" + event.Name
	})
	bus.Subscribe(func(_ context.Context, event applications.Event) {
		order <- "two:" + event.Name
	})

	bus.Publish(context.Background(), applications.Event{Name: "first"})
	awaitEventBusSignal(t, firstEntered, "first subscriber")

	// The ordered worker is blocked in the first callback. A later publication
	// must still enqueue and return rather than inheriting that callback's work.
	published := make(chan struct{})
	go func() {
		bus.Publish(context.Background(), applications.Event{Name: "second"})
		close(published)
	}()
	awaitEventBusSignal(t, published, "nonblocking publication")
	close(releaseFirst)

	got := make([]string, 0, 4)
	for range 4 {
		got = append(got, awaitEventBusValue(t, order, "ordered callback"))
	}
	want := []string{"one:first", "two:first", "one:second", "two:second"}
	if !slices.Equal(got, want) {
		t.Fatalf("dispatch order = %v, want %v", got, want)
	}
}

func TestEventBusDefensivelyCopiesPayloadForCallerAndSubscribers(t *testing.T) {
	bus := newEventBus(context.Background(), 4)
	t.Cleanup(bus.Close)
	original := []byte(`{"state":"original"}`)
	want := append([]byte(nil), original...)
	second := make(chan []byte, 1)

	bus.Subscribe(func(_ context.Context, event applications.Event) {
		event.Payload[0] = '!'
	})
	bus.Subscribe(func(_ context.Context, event applications.Event) {
		copy := append([]byte(nil), event.Payload...)
		event.Payload[len(event.Payload)-1] = '!'
		second <- copy
	})

	bus.Publish(context.Background(), applications.Event{Payload: original})
	got := awaitEventBusValue(t, second, "copied payload")
	if !slices.Equal(original, want) {
		t.Fatalf("caller payload mutated to %q, want %q", original, want)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("second subscriber received mutated payload %q, want %q", got, want)
	}
}

func TestEventBusUnsubscribeDoesNotChangeQueuedSnapshots(t *testing.T) {
	bus := newEventBus(context.Background(), 4)
	t.Cleanup(bus.Close)
	blockerEntered := make(chan struct{})
	releaseBlocker := make(chan struct{})
	received := make(chan string, 2)

	bus.Subscribe(func(_ context.Context, event applications.Event) {
		if event.Name == "queued" {
			close(blockerEntered)
			<-releaseBlocker
		}
	})
	unsubscribe := bus.Subscribe(func(_ context.Context, event applications.Event) {
		received <- event.Name
	})

	bus.Publish(context.Background(), applications.Event{Name: "queued"})
	awaitEventBusSignal(t, blockerEntered, "blocking subscriber")
	unsubscribe()
	unsubscribe()
	close(releaseBlocker)

	if got := awaitEventBusValue(t, received, "queued subscriber snapshot"); got != "queued" {
		t.Fatalf("queued delivery = %q, want queued", got)
	}
	bus.Publish(context.Background(), applications.Event{Name: "later"})
	assertNoEventBusValue(t, received, "delivery after unsubscribe")
}

func TestEventBusUsesBoundedDropNewQueue(t *testing.T) {
	bus := newEventBus(context.Background(), 2)
	t.Cleanup(bus.Close)
	entered := make(chan struct{})
	release := make(chan struct{})
	received := make(chan string, 4)

	bus.Subscribe(func(_ context.Context, event applications.Event) {
		if event.Name == "active" {
			close(entered)
			<-release
		}
		received <- event.Name
	})
	bus.Publish(context.Background(), applications.Event{Name: "active"})
	awaitEventBusSignal(t, entered, "active subscriber")
	bus.Publish(context.Background(), applications.Event{Name: "queued-one"})
	bus.Publish(context.Background(), applications.Event{Name: "queued-two"})
	bus.Publish(context.Background(), applications.Event{Name: "dropped"})
	close(release)

	got := make([]string, 0, 3)
	for range 3 {
		got = append(got, awaitEventBusValue(t, received, "accepted event"))
	}
	want := []string{"active", "queued-one", "queued-two"}
	if !slices.Equal(got, want) {
		t.Fatalf("accepted events = %v, want %v", got, want)
	}
	assertNoEventBusValue(t, received, "drop-new event")
}

func TestEventBusRecoversSubscriberPanics(t *testing.T) {
	bus := newEventBus(context.Background(), 4)
	t.Cleanup(bus.Close)
	received := make(chan string, 2)
	bus.Subscribe(func(context.Context, applications.Event) {
		panic("broken core hook")
	})
	bus.Subscribe(func(_ context.Context, event applications.Event) {
		received <- event.Name
	})

	bus.Publish(context.Background(), applications.Event{Name: "first"})
	bus.Publish(context.Background(), applications.Event{Name: "second"})
	for _, want := range []string{"first", "second"} {
		if got := awaitEventBusValue(t, received, "post-panic subscriber"); got != want {
			t.Fatalf("event after panic = %q, want %q", got, want)
		}
	}
}

func TestEventBusCloseCancelsCallbackAndWaitsForWorker(t *testing.T) {
	bus := newEventBus(context.Background(), 1)
	started := make(chan struct{})
	exited := make(chan struct{})
	bus.Subscribe(func(ctx context.Context, _ applications.Event) {
		close(started)
		<-ctx.Done()
		close(exited)
	})
	bus.Publish(context.Background(), applications.Event{Name: "active"})
	awaitEventBusSignal(t, started, "active callback")

	closed := make(chan struct{})
	go func() {
		bus.Close()
		close(closed)
	}()
	awaitEventBusSignal(t, exited, "callback cancellation")
	awaitEventBusSignal(t, closed, "bus close")

	// Close is idempotent, and publication after shutdown is ignored.
	bus.Close()
	bus.Publish(context.Background(), applications.Event{Name: "ignored"})
}

func awaitEventBusSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func awaitEventBusValue[T any](t *testing.T, values <-chan T, description string) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
		var zero T
		return zero
	}
}

func assertNoEventBusValue[T any](t *testing.T, values <-chan T, description string) {
	t.Helper()
	select {
	case value := <-values:
		t.Fatalf("unexpected %s: %v", description, value)
	case <-time.After(50 * time.Millisecond):
	}
}
