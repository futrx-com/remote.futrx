// Package lifecycle provides typed, in-process application lifecycle events.
package lifecycle

import (
	"context"
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

// UpdatePublisher owns application self-update subscribers and dispatches
// events to them synchronously in registration order.
type UpdatePublisher struct {
	events eventDispatcher[UpdateEvent]
}

// NewUpdatePublisher creates a publisher with no subscribers.
func NewUpdatePublisher() *UpdatePublisher {
	return &UpdatePublisher{}
}

// Subscribe registers a subscriber and returns an idempotent function that
// removes it.
func (p *UpdatePublisher) Subscribe(subscriber UpdateSubscriber) (unsubscribe func()) {
	return p.events.subscribe(func(ctx context.Context, event UpdateEvent) {
		subscriber.OnUpdate(ctx, event)
	})
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
	p.events.publish(ctx, event)
}
