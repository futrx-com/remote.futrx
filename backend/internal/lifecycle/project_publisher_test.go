package lifecycle

import (
	"context"
	"testing"
)

type recordingProjectSubscriber struct {
	ctx    context.Context
	events []ProjectEvent
}

func (s *recordingProjectSubscriber) OnProject(ctx context.Context, event ProjectEvent) {
	s.ctx = ctx
	s.events = append(s.events, event)
}

func TestProjectPublisherConstructsProjectEvents(t *testing.T) {
	publisher := NewProjectPublisher()
	subscriber := &recordingProjectSubscriber{}
	publisher.Subscribe(subscriber)
	ctx := context.Background()

	publisher.PublishProjectCreated(ctx, "deadbeef")
	publisher.PublishProjectUpdated(ctx, "cafebabe")
	publisher.PublishProjectDeleted(ctx, "0123abcd")

	want := []ProjectEvent{
		{State: ProjectCreated, ProjectID: "deadbeef"},
		{State: ProjectUpdated, ProjectID: "cafebabe"},
		{State: ProjectDeleted, ProjectID: "0123abcd"},
	}
	if len(subscriber.events) != len(want) {
		t.Fatalf("events = %d, want %d", len(subscriber.events), len(want))
	}
	for index := range want {
		if subscriber.events[index] != want[index] {
			t.Fatalf("event %d = %+v, want %+v", index, subscriber.events[index], want[index])
		}
	}
	if subscriber.ctx != ctx {
		t.Fatal("subscriber did not receive the publishing context")
	}
}
