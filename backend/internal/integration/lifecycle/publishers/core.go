// Package publishers holds concrete, application-wide integration
// publishers. Core is the update-lifecycle publisher: one process-wide
// instance, constructed explicitly at startup and shared with every producer
// and observer that is given it. It is generic and policy-free - it knows
// nothing about auth, 2FA, or any other producer's semantics.
package publishers

import (
	"context"
	"sync"
)

// UpdateEvent is the non-sensitive lifecycle identity delivered to
// observers. Source, Operation, and Subject are deliberately exact strings
// rather than an open metadata map, so a producer cannot accidentally widen
// what it publishes over time.
type UpdateEvent struct {
	Source    string
	Operation string
	Subject   string
}

// Observer receives update-lifecycle callbacks. Implementations are trusted
// in-process components: they must not panic, and their return values (none)
// cannot alter or mask the producer's own result.
type Observer interface {
	OnUpdateStarted(context.Context, UpdateEvent)
	OnUpdateCompleted(context.Context, UpdateEvent)
	OnUpdateFailed(context.Context, UpdateEvent, error)
}

type subscription struct {
	id       uint64
	observer Observer
}

// Core is the concrete, concurrency-safe update-lifecycle publisher. It owns
// the observer registry and dispatches events to subscribers synchronously,
// in registration order.
type Core struct {
	mu     sync.RWMutex
	nextID uint64
	subs   []subscription
}

// New creates an empty Core. Callers construct exactly one instance at
// startup and share it explicitly; there is no package-level default.
func New() *Core {
	return &Core{}
}

// Subscribe registers observer and returns a function that removes it.
// Calling the returned function more than once is a no-op.
func (c *Core) Subscribe(observer Observer) (unsubscribe func()) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.subs = append(c.subs, subscription{id: id, observer: observer})
	c.mu.Unlock()

	removed := false
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if removed {
			return
		}
		removed = true
		for i, sub := range c.subs {
			if sub.id == id {
				c.subs = append(c.subs[:i], c.subs[i+1:]...)
				break
			}
		}
	}
}

// snapshot copies the current subscriber list under a read lock and returns
// it, so dispatch can run every callback with the lock released - an
// observer may then subscribe or unsubscribe from inside its own callback
// without deadlocking.
func (c *Core) snapshot() []subscription {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]subscription, len(c.subs))
	copy(out, c.subs)
	return out
}

// PublishUpdateStarted notifies every current subscriber that source is
// beginning operation against subject.
func (c *Core) PublishUpdateStarted(ctx context.Context, source, operation, subject string) {
	event := UpdateEvent{Source: source, Operation: operation, Subject: subject}
	for _, sub := range c.snapshot() {
		sub.observer.OnUpdateStarted(ctx, event)
	}
}

// PublishUpdateCompleted notifies every current subscriber that source
// finished operation against subject successfully.
func (c *Core) PublishUpdateCompleted(ctx context.Context, source, operation, subject string) {
	event := UpdateEvent{Source: source, Operation: operation, Subject: subject}
	for _, sub := range c.snapshot() {
		sub.observer.OnUpdateCompleted(ctx, event)
	}
}

// PublishUpdateFailed notifies every current subscriber that source's
// operation against subject failed with err.
func (c *Core) PublishUpdateFailed(ctx context.Context, source, operation, subject string, err error) {
	event := UpdateEvent{Source: source, Operation: operation, Subject: subject}
	for _, sub := range c.snapshot() {
		sub.observer.OnUpdateFailed(ctx, event, err)
	}
}
