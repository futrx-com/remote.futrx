package lifecycle

import (
	"context"
	"sync"
	"testing"
)

type recordingApplicationCatalogSubscriber struct {
	name   string
	mu     *sync.Mutex
	events *[]recordedApplicationCatalogEvent
}

type recordedApplicationCatalogEvent struct {
	subscriber string
	ctx        context.Context
	event      ApplicationCatalogEvent
}

func (s recordingApplicationCatalogSubscriber) OnApplicationCatalog(
	ctx context.Context,
	event ApplicationCatalogEvent,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	*s.events = append(*s.events, recordedApplicationCatalogEvent{
		subscriber: s.name,
		ctx:        ctx,
		event:      event,
	})
}

type recordingApplicationInstanceSubscriber struct {
	ctx    context.Context
	events []ApplicationInstanceEvent
}

func (s *recordingApplicationInstanceSubscriber) OnApplicationInstance(
	ctx context.Context,
	event ApplicationInstanceEvent,
) {
	s.ctx = ctx
	s.events = append(s.events, event)
}

func TestApplicationPublisherConstructsCatalogEventsInRegistrationOrder(t *testing.T) {
	publisher := NewApplicationPublisher()
	var mu sync.Mutex
	var events []recordedApplicationCatalogEvent
	publisher.SubscribeCatalog(recordingApplicationCatalogSubscriber{
		name: "first", mu: &mu, events: &events,
	})
	publisher.SubscribeCatalog(recordingApplicationCatalogSubscriber{
		name: "second", mu: &mu, events: &events,
	})
	ctx := context.Background()

	publisher.PublishApplicationAdded(ctx, "hello-remote")
	publisher.PublishApplicationUpdated(ctx, "hello-remote")
	publisher.PublishApplicationDeleted(ctx, "hello-remote")

	wantStates := []ApplicationCatalogState{
		ApplicationAdded,
		ApplicationUpdated,
		ApplicationDeleted,
	}
	if len(events) != len(wantStates)*2 {
		t.Fatalf("events = %d, want %d", len(events), len(wantStates)*2)
	}
	for index, state := range wantStates {
		for subscriberOffset, subscriber := range []string{"first", "second"} {
			got := events[index*2+subscriberOffset]
			if got.subscriber != subscriber {
				t.Fatalf("event %d subscriber = %q, want %q", index, got.subscriber, subscriber)
			}
			if got.event != (ApplicationCatalogEvent{State: state, ApplicationID: "hello-remote"}) {
				t.Fatalf("event %d = %+v", index, got.event)
			}
			if got.ctx != ctx {
				t.Fatal("subscriber did not receive the publishing context")
			}
		}
	}
}

func TestApplicationPublisherConstructsInstanceEvents(t *testing.T) {
	publisher := NewApplicationPublisher()
	subscriber := &recordingApplicationInstanceSubscriber{}
	publisher.SubscribeInstances(subscriber)
	ctx := context.Background()

	publisher.PublishApplicationInstalled(ctx, "hello-remote", "instance-1", "project", "project-1")
	publisher.PublishApplicationStopped(ctx, "hello-remote", "instance-1", "project", "project-1")
	publisher.PublishApplicationStarted(ctx, "hello-remote", "instance-1", "project", "project-1")
	publisher.PublishApplicationUninstalled(ctx, "hello-remote", "instance-1", "project", "project-1")

	wantStates := []ApplicationInstanceState{
		ApplicationInstalled,
		ApplicationStopped,
		ApplicationStarted,
		ApplicationUninstalled,
	}
	if len(subscriber.events) != len(wantStates) {
		t.Fatalf("events = %d, want %d", len(subscriber.events), len(wantStates))
	}
	for index, state := range wantStates {
		want := ApplicationInstanceEvent{
			State:         state,
			ApplicationID: "hello-remote",
			InstanceID:    "instance-1",
			Scope:         "project",
			ProjectID:     "project-1",
		}
		if subscriber.events[index] != want {
			t.Fatalf("event %d = %+v, want %+v", index, subscriber.events[index], want)
		}
	}
	if subscriber.ctx != ctx {
		t.Fatal("subscriber did not receive the publishing context")
	}
}

func TestApplicationPublisherSubscriptionsAreIndependentAndRemovable(t *testing.T) {
	publisher := NewApplicationPublisher()
	var mu sync.Mutex
	var catalogEvents []recordedApplicationCatalogEvent
	unsubscribe := publisher.SubscribeCatalog(recordingApplicationCatalogSubscriber{
		mu: &mu, events: &catalogEvents,
	})
	instances := &recordingApplicationInstanceSubscriber{}
	publisher.SubscribeInstances(instances)

	publisher.PublishApplicationAdded(context.Background(), "demo")
	unsubscribe()
	unsubscribe()
	publisher.PublishApplicationDeleted(context.Background(), "demo")
	publisher.PublishApplicationInstalled(context.Background(), "demo", "one", "global", "")

	if len(catalogEvents) != 1 {
		t.Fatalf("catalog events = %d, want only the event before unsubscribe", len(catalogEvents))
	}
	if len(instances.events) != 1 || instances.events[0].State != ApplicationInstalled {
		t.Fatalf("instance events = %+v", instances.events)
	}
}
