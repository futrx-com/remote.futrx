package lifecycle

import (
	"context"
	"sync"
	"testing"
	"time"
)

type recordingUpdateSubscriber struct {
	name   string
	mu     *sync.Mutex
	events *[]recordedUpdateEvent
}

type recordedUpdateEvent struct {
	subscriber string
	ctx        context.Context
	event      UpdateEvent
}

func (s recordingUpdateSubscriber) OnUpdate(ctx context.Context, event UpdateEvent) {
	s.mu.Lock()
	*s.events = append(*s.events, recordedUpdateEvent{
		subscriber: s.name,
		ctx:        ctx,
		event:      event,
	})
	s.mu.Unlock()
}

func TestUpdatePublisherDispatchesInRegistrationOrder(t *testing.T) {
	publisher := NewUpdatePublisher()
	var mu sync.Mutex
	var events []recordedUpdateEvent
	publisher.Subscribe(recordingUpdateSubscriber{name: "first", mu: &mu, events: &events})
	publisher.Subscribe(recordingUpdateSubscriber{name: "second", mu: &mu, events: &events})

	ctx := context.Background()
	publisher.PublishUpdateStarted(ctx, "0.4.0", "infrastructure", "admin@example.com")

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].subscriber != "first" || events[1].subscriber != "second" {
		t.Fatalf("subscriber order = %q, %q", events[0].subscriber, events[1].subscriber)
	}
	want := UpdateEvent{
		State: UpdateStarted, Target: "0.4.0", Kind: "infrastructure", StartedBy: "admin@example.com",
	}
	for _, got := range events {
		if got.event != want {
			t.Fatalf("event = %+v, want %+v", got.event, want)
		}
		if got.ctx != ctx {
			t.Fatal("subscriber did not receive the publishing context")
		}
	}
}

func TestUpdatePublisherIdentifiesTerminalStates(t *testing.T) {
	publisher := NewUpdatePublisher()
	var mu sync.Mutex
	var events []recordedUpdateEvent
	publisher.Subscribe(recordingUpdateSubscriber{mu: &mu, events: &events})

	ctx := context.Background()
	publisher.PublishUpdateSucceeded(ctx, "0.4.0", "application", "admin@example.com")
	publisher.PublishUpdateFailed(ctx, "0.5.0", "infrastructure", "other@example.com")

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].event != (UpdateEvent{
		State: UpdateSucceeded, Target: "0.4.0", Kind: "application", StartedBy: "admin@example.com",
	}) {
		t.Fatalf("succeeded event = %+v", events[0].event)
	}
	if events[1].event != (UpdateEvent{
		State: UpdateFailed, Target: "0.5.0", Kind: "infrastructure", StartedBy: "other@example.com",
	}) {
		t.Fatalf("failed event = %+v", events[1].event)
	}
	if events[0].ctx != ctx || events[1].ctx != ctx {
		t.Fatal("terminal subscribers did not receive the publishing context")
	}
}

func TestUpdatePublisherDispatchesCanceledContext(t *testing.T) {
	publisher := NewUpdatePublisher()
	var received context.Context
	publisher.Subscribe(updateSubscriberFunc(func(ctx context.Context, _ UpdateEvent) {
		received = ctx
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	publisher.PublishUpdateStarted(ctx, "0.4.0", "application", "admin@example.com")

	if received != ctx {
		t.Fatal("subscriber did not receive the canceled publishing context")
	}
	if received.Err() != context.Canceled {
		t.Fatalf("subscriber context error = %v, want context canceled", received.Err())
	}
}

func TestUpdatePublisherPropagatesPanicAndSkipsLaterSubscribers(t *testing.T) {
	publisher := NewUpdatePublisher()
	publisher.Subscribe(updateSubscriberFunc(func(context.Context, UpdateEvent) {
		panic("subscriber failed")
	}))
	laterCalled := false
	publisher.Subscribe(updateSubscriberFunc(func(context.Context, UpdateEvent) {
		laterCalled = true
	}))

	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		publisher.PublishUpdateStarted(context.Background(), "0.4.0", "application", "admin@example.com")
	}()

	if recovered != "subscriber failed" {
		t.Fatalf("recovered panic = %v, want subscriber failure", recovered)
	}
	if laterCalled {
		t.Fatal("subscriber after panic was called")
	}
}

func TestUpdatePublisherUnsubscribeIsIdempotent(t *testing.T) {
	publisher := NewUpdatePublisher()
	var mu sync.Mutex
	var events []recordedUpdateEvent
	unsubscribe := publisher.Subscribe(recordingUpdateSubscriber{mu: &mu, events: &events})

	publisher.PublishUpdateStarted(context.Background(), "0.4.0", "application", "admin@example.com")
	unsubscribe()
	unsubscribe()
	publisher.PublishUpdateStarted(context.Background(), "0.4.1", "application", "admin@example.com")

	if len(events) != 1 {
		t.Fatalf("events = %d, want only the event published before unsubscribe", len(events))
	}
}

func TestUpdateSubscriberMayChangeSubscriptionsDuringDispatch(t *testing.T) {
	publisher := NewUpdatePublisher()
	done := make(chan struct{})
	publisher.Subscribe(updateSubscriberFunc(func(context.Context, UpdateEvent) {
		unsubscribe := publisher.Subscribe(updateSubscriberFunc(func(context.Context, UpdateEvent) {}))
		unsubscribe()
		close(done)
	}))

	publisher.PublishUpdateStarted(context.Background(), "0.4.0", "application", "admin@example.com")
	<-done
}

func TestUpdatePublisherAllowsConcurrentDispatch(t *testing.T) {
	publisher := NewUpdatePublisher()
	callbackStarted := make(chan struct{}, 2)
	releaseCallbacks := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCallbacks) }) }
	defer release()
	publisher.Subscribe(updateSubscriberFunc(func(context.Context, UpdateEvent) {
		callbackStarted <- struct{}{}
		<-releaseCallbacks
	}))

	published := make(chan struct{}, 2)
	for range 2 {
		go func() {
			publisher.PublishUpdateStarted(context.Background(), "0.4.0", "application", "admin@example.com")
			published <- struct{}{}
		}()
	}

	for range 2 {
		select {
		case <-callbackStarted:
		case <-time.After(time.Second):
			t.Fatal("concurrent publish calls did not enter the subscriber concurrently")
		}
	}
	release()
	for range 2 {
		<-published
	}
}

func TestUnsubscribeDoesNotCancelSnapshottedDelivery(t *testing.T) {
	publisher := NewUpdatePublisher()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	defer release()
	publisher.Subscribe(updateSubscriberFunc(func(context.Context, UpdateEvent) {
		close(firstStarted)
		<-releaseFirst
	}))

	secondCalled := make(chan struct{})
	unsubscribeSecond := publisher.Subscribe(updateSubscriberFunc(func(context.Context, UpdateEvent) {
		close(secondCalled)
	}))

	published := make(chan struct{})
	go func() {
		publisher.PublishUpdateStarted(context.Background(), "0.4.0", "application", "admin@example.com")
		close(published)
	}()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first subscriber was not called")
	}
	unsubscribeSecond()
	release()

	select {
	case <-secondCalled:
	case <-time.After(time.Second):
		t.Fatal("unsubscribe suppressed a delivery already captured by the dispatch snapshot")
	}
	<-published
}

type updateSubscriberFunc func(context.Context, UpdateEvent)

func (subscriber updateSubscriberFunc) OnUpdate(ctx context.Context, event UpdateEvent) {
	subscriber(ctx, event)
}
