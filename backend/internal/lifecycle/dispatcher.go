package lifecycle

import (
	"context"
	"sync"
)

type eventSubscription[Event any] struct {
	id     uint64
	notify func(context.Context, Event)
}

// eventDispatcher owns the delivery mechanics shared by every lifecycle
// family. Public publishers keep event construction and subscriber contracts
// domain-specific.
type eventDispatcher[Event any] struct {
	mu            sync.RWMutex
	nextID        uint64
	subscriptions []eventSubscription[Event]
}

func (d *eventDispatcher[Event]) subscribe(notify func(context.Context, Event)) (unsubscribe func()) {
	d.mu.Lock()
	id := d.nextID
	d.nextID++
	d.subscriptions = append(d.subscriptions, eventSubscription[Event]{id: id, notify: notify})
	d.mu.Unlock()

	removed := false
	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		if removed {
			return
		}
		removed = true
		for index, subscription := range d.subscriptions {
			if subscription.id == id {
				d.subscriptions = append(d.subscriptions[:index], d.subscriptions[index+1:]...)
				return
			}
		}
	}
}

func (d *eventDispatcher[Event]) publish(ctx context.Context, event Event) {
	for _, subscription := range d.snapshot() {
		subscription.notify(ctx, event)
	}
}

func (d *eventDispatcher[Event]) snapshot() []eventSubscription[Event] {
	d.mu.RLock()
	defer d.mu.RUnlock()
	subscriptions := make([]eventSubscription[Event], len(d.subscriptions))
	copy(subscriptions, d.subscriptions)
	return subscriptions
}
