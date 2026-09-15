// Package lifecycle provides typed, in-process application lifecycle events.
package lifecycle

import (
	"context"
	"sync"
)

// UpdateState identifies the transition represented by an UpdateEvent.
type UpdateState string

const (
	UpdateStarted   UpdateState = "started"
	UpdateSucceeded UpdateState = "succeeded"
	UpdateFailed    UpdateState = "failed"
)

// UpdateEvent is the non-sensitive identity of an application self-update
// transition.
type UpdateEvent struct {
	State     UpdateState
	Target    string
	Kind      string
	StartedBy string
}

// UpdateSubscriber receives application self-update lifecycle events.
type UpdateSubscriber interface {
	OnUpdate(context.Context, UpdateEvent)
}

type updateSubscription struct {
	id         uint64
	subscriber UpdateSubscriber
}

// UpdatePublisher owns application self-update subscribers and dispatches
// events to them synchronously in registration order.
type UpdatePublisher struct {
	mu            sync.RWMutex
	nextID        uint64
	subscriptions []updateSubscription
}

// NewUpdatePublisher creates a publisher with no subscribers.
func NewUpdatePublisher() *UpdatePublisher {
	return &UpdatePublisher{}
}

// Subscribe registers a subscriber and returns an idempotent function that
// removes it.
func (p *UpdatePublisher) Subscribe(subscriber UpdateSubscriber) (unsubscribe func()) {
	p.mu.Lock()
	id := p.nextID
	p.nextID++
	p.subscriptions = append(p.subscriptions, updateSubscription{id: id, subscriber: subscriber})
	p.mu.Unlock()

	removed := false
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if removed {
			return
		}
		removed = true
		for index, subscription := range p.subscriptions {
			if subscription.id == id {
				p.subscriptions = append(p.subscriptions[:index], p.subscriptions[index+1:]...)
				return
			}
		}
	}
}

// PublishUpdateStarted reports that an application update has started.
func (p *UpdatePublisher) PublishUpdateStarted(ctx context.Context, target, kind, startedBy string) {
	p.publish(ctx, UpdateEvent{
		State: UpdateStarted, Target: target, Kind: kind, StartedBy: startedBy,
	})
}

// PublishUpdateSucceeded reports that an application update completed
// successfully.
func (p *UpdatePublisher) PublishUpdateSucceeded(ctx context.Context, target, kind, startedBy string) {
	p.publish(ctx, UpdateEvent{
		State: UpdateSucceeded, Target: target, Kind: kind, StartedBy: startedBy,
	})
}

// PublishUpdateFailed reports that an application update terminated
// unsuccessfully.
func (p *UpdatePublisher) PublishUpdateFailed(ctx context.Context, target, kind, startedBy string) {
	p.publish(ctx, UpdateEvent{
		State: UpdateFailed, Target: target, Kind: kind, StartedBy: startedBy,
	})
}

func (p *UpdatePublisher) publish(ctx context.Context, event UpdateEvent) {
	for _, subscription := range p.snapshot() {
		subscription.subscriber.OnUpdate(ctx, event)
	}
}

func (p *UpdatePublisher) snapshot() []updateSubscription {
	p.mu.RLock()
	defer p.mu.RUnlock()
	subscriptions := make([]updateSubscription, len(p.subscriptions))
	copy(subscriptions, p.subscriptions)
	return subscriptions
}
