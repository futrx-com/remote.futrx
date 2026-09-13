package lifecycle

import "context"

// ProjectState identifies a persisted project lifecycle transition.
type ProjectState string

const (
	ProjectCreated ProjectState = "created"
	ProjectUpdated ProjectState = "updated"
	ProjectDeleted ProjectState = "deleted"
)

// ProjectEvent identifies a persisted project lifecycle transition without
// exposing project configuration, workspace paths, or secrets.
type ProjectEvent struct {
	State     ProjectState
	ProjectID string
}

// ProjectSubscriber receives project lifecycle events.
type ProjectSubscriber interface {
	OnProject(context.Context, ProjectEvent)
}

// ProjectPublisher owns project lifecycle subscribers and dispatches events
// to them synchronously in registration order.
type ProjectPublisher struct {
	events eventDispatcher[ProjectEvent]
}

// NewProjectPublisher creates a publisher with no subscribers.
func NewProjectPublisher() *ProjectPublisher {
	return &ProjectPublisher{}
}

// Subscribe registers a subscriber and returns an idempotent function that
// removes it.
func (p *ProjectPublisher) Subscribe(subscriber ProjectSubscriber) (unsubscribe func()) {
	return p.events.subscribe(func(ctx context.Context, event ProjectEvent) {
		subscriber.OnProject(ctx, event)
	})
}

// PublishProjectCreated reports that a project record was created.
func (p *ProjectPublisher) PublishProjectCreated(ctx context.Context, projectID string) {
	p.publish(ctx, ProjectCreated, projectID)
}

// PublishProjectUpdated reports that a project record was updated.
func (p *ProjectPublisher) PublishProjectUpdated(ctx context.Context, projectID string) {
	p.publish(ctx, ProjectUpdated, projectID)
}

// PublishProjectDeleted reports that a project record was deleted.
func (p *ProjectPublisher) PublishProjectDeleted(ctx context.Context, projectID string) {
	p.publish(ctx, ProjectDeleted, projectID)
}

func (p *ProjectPublisher) publish(ctx context.Context, state ProjectState, projectID string) {
	p.events.publish(ctx, ProjectEvent{State: state, ProjectID: projectID})
}
