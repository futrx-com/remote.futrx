package lifecycle

import (
	"context"
	"log"
	"sync"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

const applicationEventBusQueueCapacity = 256

// EventBus dispatches dynamically named application events inside the Remote
// process. Delivery is in-memory, bounded, and asynchronous so a core callback
// can never run inline under a producer-owned lifecycle or instance lock. It
// deliberately provides no persistence, retry, or cross-process transport.
type EventBus struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	queue  chan queuedApplicationEvent
	close  sync.Once
	events eventDispatcher[applications.Event]
}

type queuedApplicationEvent struct {
	event         applications.Event
	subscriptions []eventSubscription[applications.Event]
}

// NewEventBus creates an application event bus whose worker lives until ctx is
// canceled or Close is called.
func NewEventBus(ctx context.Context) *EventBus {
	return newEventBus(ctx, applicationEventBusQueueCapacity)
}

func newEventBus(parent context.Context, capacity int) *EventBus {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	bus := &EventBus{
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		queue:  make(chan queuedApplicationEvent, capacity),
	}
	go bus.run()
	return bus
}

// Publish snapshots the current subscribers and nonblockingly queues a copied
// event. One worker dispatches accepted events and their subscriber snapshots
// in order. If the bounded queue is full, the new event is dropped and logged;
// events already accepted retain their order.
//
// The publishing context is intentionally not retained: it may be canceled as
// soon as Publish returns. Subscribers receive the bus lifecycle context.
func (b *EventBus) Publish(_ context.Context, event applications.Event) {
	if b.ctx.Err() != nil {
		return
	}
	delivery := queuedApplicationEvent{
		event:         cloneApplicationEvent(event),
		subscriptions: b.events.snapshot(),
	}
	if len(delivery.subscriptions) == 0 {
		return
	}
	select {
	case <-b.ctx.Done():
		return
	case b.queue <- delivery:
	default:
		log.Printf(
			"lifecycle: application event queue full; dropping %s event %s",
			event.Source.Publisher,
			event.Name,
		)
	}
}

// Subscribe registers a callback and returns an idempotent function that
// removes it. A queued event retains the subscriber snapshot captured when it
// was published, even if a callback later unsubscribes.
func (b *EventBus) Subscribe(
	subscriber func(context.Context, applications.Event),
) (unsubscribe func()) {
	return b.events.subscribe(func(ctx context.Context, event applications.Event) {
		subscriber(ctx, cloneApplicationEvent(event))
	})
}

// Close stops accepting events, cancels the context given to callbacks, and
// waits for the ordered dispatch worker to exit. It is safe to call more than
// once. Subscribers must honor context cancellation and must not call Close
// from inside their own callback.
func (b *EventBus) Close() {
	b.close.Do(b.cancel)
	<-b.done
}

func (b *EventBus) run() {
	defer close(b.done)
	for {
		if b.ctx.Err() != nil {
			return
		}
		select {
		case <-b.ctx.Done():
			return
		case delivery := <-b.queue:
			for _, subscription := range delivery.subscriptions {
				if b.ctx.Err() != nil {
					return
				}
				b.notify(subscription, delivery.event)
			}
		}
	}
}

func (b *EventBus) notify(
	subscription eventSubscription[applications.Event],
	event applications.Event,
) {
	defer func() {
		if failure := recover(); failure != nil {
			log.Printf(
				"lifecycle: application event subscriber %d panicked handling %s event %s: %v",
				subscription.id,
				event.Source.Publisher,
				event.Name,
				failure,
			)
		}
	}()
	subscription.notify(b.ctx, event)
}

func cloneApplicationEvent(event applications.Event) applications.Event {
	event.Payload = append(event.Payload[:0:0], event.Payload...)
	return event
}
