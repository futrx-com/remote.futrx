package applications

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	applicationapi "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type eventTestRegistry map[string]Application

func (r eventTestRegistry) List() []Application {
	applications := make([]Application, 0, len(r))
	for _, application := range r {
		applications = append(applications, application)
	}
	return applications
}

func (r eventTestRegistry) Get(id string) (Application, bool) {
	application, ok := r[id]
	return application, ok
}

func (eventTestRegistry) UIAsset(string, string) ([]byte, bool) { return nil, false }

type testEventSource struct {
	mu           sync.Mutex
	subscriber   func(context.Context, applicationapi.Event)
	unsubscribed chan struct{}
	once         sync.Once
}

func newTestEventSource() *testEventSource {
	return &testEventSource{unsubscribed: make(chan struct{})}
}

func (s *testEventSource) Subscribe(
	subscriber func(context.Context, applicationapi.Event),
) func() {
	s.mu.Lock()
	s.subscriber = subscriber
	s.mu.Unlock()
	return func() {
		s.once.Do(func() { close(s.unsubscribed) })
		s.mu.Lock()
		s.subscriber = nil
		s.mu.Unlock()
	}
}

func (s *testEventSource) publish(event applicationapi.Event) {
	s.mu.Lock()
	subscriber := s.subscriber
	s.mu.Unlock()
	if subscriber != nil {
		subscriber(context.Background(), event)
	}
}

type routedNotification struct {
	instance  applicationapi.Instance
	event     applicationapi.Event
	deadline  bool
	remaining time.Duration
}

type routingHost struct {
	mu            sync.Mutex
	notifications []routedNotification
	notified      chan routedNotification
	errors        map[string]error
	blocks        map[string]<-chan struct{}
	ensureCalls   int
}

func newRoutingHost() *routingHost {
	return &routingHost{notified: make(chan routedNotification, 32)}
}

func (h *routingHost) Ensure(
	_ context.Context,
	instance applicationapi.Instance,
) (applicationapi.Descriptor, error) {
	h.mu.Lock()
	h.ensureCalls++
	h.mu.Unlock()
	return applicationapi.Descriptor{
		Name: instance.ApplicationName, APIVersion: applicationapi.APIVersion,
	}, nil
}

func (*routingHost) Call(
	context.Context,
	applicationapi.Instance,
	applicationapi.Request,
) (applicationapi.Response, error) {
	return applicationapi.Response{}, nil
}

func (h *routingHost) Notify(
	ctx context.Context,
	instance applicationapi.Instance,
	event applicationapi.Event,
) error {
	deadlineAt, deadline := ctx.Deadline()
	notification := routedNotification{
		instance: instance,
		event:    cloneRoutedEvent(event),
		deadline: deadline,
	}
	if deadline {
		notification.remaining = time.Until(deadlineAt)
	}
	h.mu.Lock()
	h.notifications = append(h.notifications, notification)
	h.mu.Unlock()
	h.notified <- notification
	if block := h.blocks[instance.ID]; block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return h.errors[instance.ID]
}

func (*routingHost) Stop(context.Context, string) error   { return nil }
func (*routingHost) Remove(context.Context, string) error { return nil }
func (*routingHost) InvalidateApplication(string)         {}

func (h *routingHost) ensureCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ensureCalls
}

func TestEventRouterMatchesExactSubscriptionsAndSkipsStoppedInstances(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	source := newTestEventSource()
	host := newRoutingHost()
	registry := eventTestRegistry{
		"matching": subscriberApplication(
			"matching", "remote.applications", "installed"),
		"wrong-event": subscriberApplication(
			"wrong-event", "remote.applications", "stopped"),
		"wrong-publisher": subscriberApplication(
			"wrong-publisher", "applications.other.events", "installed"),
		"stopped": subscriberApplication(
			"stopped", "remote.applications", "installed"),
	}
	store := &fakeStore{global: []Instance{
		runningApplicationInstance("match-1", "matching"),
		runningApplicationInstance("wrong-event-1", "wrong-event"),
		runningApplicationInstance("wrong-publisher-1", "wrong-publisher"),
		{ID: "stopped-1", ApplicationID: "stopped", Scope: ScopeGlobal, Status: StatusStopped},
	}}
	newEventRoutingService(ctx, registry, store, host, source)

	payload := []byte(`{"applicationId":"demo"}`)
	source.publish(applicationapi.Event{
		Source:  applicationapi.EventSource{ApplicationID: "remote", Publisher: "remote.applications"},
		Name:    "installed",
		Version: 1,
		Payload: payload,
	})
	payload[0] = '['

	notification := awaitNotification(t, host.notified)
	if notification.instance.ID != "match-1" {
		t.Fatalf("notified instance = %q, want match-1", notification.instance.ID)
	}
	if string(notification.event.Payload) != `{"applicationId":"demo"}` {
		t.Fatalf("payload = %s, want an enqueue-time copy", notification.event.Payload)
	}
	if !notification.deadline {
		t.Fatal("backend notification has no application timeout")
	}
	assertNoNotification(t, host.notified)
	if host.ensureCount() != 0 {
		t.Fatalf("router called Ensure %d times; Notify owns lazy startup", host.ensureCount())
	}
}

func TestEventRouterNameSubscriptionsDeliverEveryEventVersion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	source := newTestEventSource()
	host := newRoutingHost()
	registry := eventTestRegistry{
		"consumer": subscriberApplication(
			"consumer",
			applicationapi.RemoteApplicationsPublisher,
			applicationapi.RemoteApplicationInstalled,
		),
	}
	store := &fakeStore{global: []Instance{
		runningApplicationInstance("consumer-1", "consumer"),
	}}
	newEventRoutingService(ctx, registry, store, host, source)

	source.publish(applicationapi.Event{
		Source: applicationapi.EventSource{
			ApplicationID: "remote",
			Publisher:     applicationapi.RemoteApplicationsPublisher,
		},
		Name:    applicationapi.RemoteApplicationInstalled,
		Version: 2,
	})

	notification := awaitNotification(t, host.notified)
	if notification.event.Version != 2 {
		t.Fatalf("delivered version = %d, want 2", notification.event.Version)
	}
}

func TestEventRouterDeliversRecipientsInInstanceIDOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	source := newTestEventSource()
	host := newRoutingHost()
	application := subscriberApplication("consumer", "remote.applications", "installed")
	store := &fakeStore{global: []Instance{
		runningApplicationInstance("z-copy", "consumer"),
		runningApplicationInstance("a-copy", "consumer"),
		runningApplicationInstance("m-copy", "consumer"),
	}}
	newEventRoutingService(
		ctx, eventTestRegistry{"consumer": application}, store, host, source)

	source.publish(applicationapi.Event{
		Source: applicationapi.EventSource{
			ApplicationID: "remote", Publisher: "remote.applications",
		},
		Name: "installed", Version: 1,
	})

	got := make([]string, 0, 3)
	for range 3 {
		got = append(got, awaitNotification(t, host.notified).instance.ID)
	}
	if want := []string{"a-copy", "m-copy", "z-copy"}; !equalStrings(got, want) {
		t.Fatalf("delivery order = %v, want %v", got, want)
	}
}

func TestEventRouterKeepsProjectEventsInsideTheirProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	source := newTestEventSource()
	host := newRoutingHost()
	application := subscriberApplication(
		"consumer", "applications.producer.activity", "changed")
	store := &fakeStore{
		global: []Instance{runningApplicationInstance("global", "consumer")},
		byProject: map[string][]Instance{
			"project-a": {runningProjectApplicationInstance("project-a-copy", "consumer", "project-a")},
			"project-b": {runningProjectApplicationInstance("project-b-copy", "consumer", "project-b")},
		},
	}
	newEventRoutingService(
		ctx, eventTestRegistry{"consumer": application}, store, host, source)

	source.publish(applicationapi.Event{
		Source: applicationapi.EventSource{
			ApplicationID: "producer",
			InstanceID:    "producer-a",
			Scope:         string(ScopeProject),
			ProjectID:     "project-a",
			Publisher:     "applications.producer.activity",
		},
		Name: "changed", Version: 1,
	})
	if got := awaitNotification(t, host.notified).instance.ID; got != "project-a-copy" {
		t.Fatalf("project event reached %q, want project-a-copy", got)
	}
	assertNoNotification(t, host.notified)

	// A global source applies server-wide, including matching project installs.
	source.publish(applicationapi.Event{
		Source: applicationapi.EventSource{
			ApplicationID: "producer",
			InstanceID:    "producer-global",
			Scope:         string(ScopeGlobal),
			Publisher:     "applications.producer.activity",
		},
		Name: "changed", Version: 1,
	})
	got := map[string]bool{}
	for range 3 {
		got[awaitNotification(t, host.notified).instance.ID] = true
	}
	for _, id := range []string{"global", "project-a-copy", "project-b-copy"} {
		if !got[id] {
			t.Errorf("global event did not reach %s; reached %v", id, got)
		}
	}
}

func TestEventRouterRejectsInconsistentOriginScope(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	source := newTestEventSource()
	host := newRoutingHost()
	application := subscriberApplication("consumer", "remote.applications", "installed")
	store := &fakeStore{global: []Instance{
		runningApplicationInstance("global", "consumer"),
	}}
	newEventRoutingService(
		ctx, eventTestRegistry{"consumer": application}, store, host, source)

	for _, origin := range []applicationapi.EventSource{
		{ApplicationID: "remote", ProjectID: "project-a", Publisher: "remote.applications"},
		{ApplicationID: "remote", Scope: string(ScopeGlobal), ProjectID: "project-a", Publisher: "remote.applications"},
		{ApplicationID: "remote", Scope: string(ScopeProject), Publisher: "remote.applications"},
		{ApplicationID: "remote", Scope: "unknown", Publisher: "remote.applications"},
	} {
		source.publish(applicationapi.Event{Source: origin, Name: "installed", Version: 1})
	}
	assertNoNotification(t, host.notified)
}

func TestEventRouterIsolatesBackendFailuresAndTimeouts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	source := newTestEventSource()
	host := newRoutingHost()
	host.errors = map[string]error{"failing": errors.New("subscriber failed")}
	host.blocks = map[string]<-chan struct{}{"timed-out": make(chan struct{})}
	registry := eventTestRegistry{
		"fails": subscriberApplication("fails", "remote.applications", "started"),
		"times-out": subscriberApplicationWithTimeout(
			"times-out", "remote.applications", "started", 10),
		"healthy": subscriberApplication("healthy", "remote.applications", "started"),
	}
	store := &fakeStore{global: []Instance{
		runningApplicationInstance("failing", "fails"),
		runningApplicationInstance("timed-out", "times-out"),
		runningApplicationInstance("z-healthy", "healthy"),
	}}
	newEventRoutingService(ctx, registry, store, host, source)

	published := make(chan struct{})
	go func() {
		source.publish(applicationapi.Event{
			Source: applicationapi.EventSource{ApplicationID: "remote", Publisher: "remote.applications"},
			Name:   "started", Version: 1,
		})
		close(published)
	}()
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("event publication waited on a backend subscriber")
	}

	got := make([]string, 0, 3)
	for range 3 {
		got = append(got, awaitNotification(t, host.notified).instance.ID)
	}
	if want := []string{"failing", "timed-out", "z-healthy"}; !equalStrings(got, want) {
		t.Fatalf("delivery order = %v, want %v", got, want)
	}
}

func TestEventRouterCapsDeliveryBelowLongBackendTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	source := newTestEventSource()
	host := newRoutingHost()
	application := subscriberApplicationWithTimeout(
		"consumer",
		"remote.applications",
		"started",
		MaxBackendTimeoutMS,
	)
	store := &fakeStore{global: []Instance{
		runningApplicationInstance("consumer-1", "consumer"),
	}}
	newEventRoutingService(
		ctx,
		eventTestRegistry{"consumer": application},
		store,
		host,
		source,
	)

	source.publish(applicationapi.Event{
		Source: applicationapi.EventSource{
			ApplicationID: "remote",
			Publisher:     "remote.applications",
		},
		Name: "started", Version: 1,
	})
	notification := awaitNotification(t, host.notified)
	maximum := time.Duration(MaxEventDeliveryTimeoutMS) * time.Millisecond
	if notification.remaining > maximum || notification.remaining < maximum-time.Second {
		t.Fatalf(
			"delivery deadline remaining = %v, want approximately %v",
			notification.remaining,
			maximum,
		)
	}
}

func TestEventRouterPassesCompleteManifestDetailsToLazyNotify(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	source := newTestEventSource()
	host := newRoutingHost()
	application := subscriberApplication("consumer", "remote.applications", "deleted")
	application.Publishers = []applicationapi.PublisherDeclaration{{
		Name: "audit",
		Events: []applicationapi.EventDeclaration{{
			Name: "recorded", Version: 1,
		}},
	}}
	store := &fakeStore{global: []Instance{runningApplicationInstance("consumer-1", "consumer")}}
	newEventRoutingService(
		ctx, eventTestRegistry{"consumer": application}, store, host, source)

	source.publish(applicationapi.Event{
		Source: applicationapi.EventSource{ApplicationID: "remote", Publisher: "remote.applications"},
		Name:   "deleted", Version: 1,
	})
	notification := awaitNotification(t, host.notified)
	if host.ensureCount() != 0 {
		t.Fatal("the service eagerly ensured a backend instead of using Notify")
	}
	if len(notification.instance.Publishers) != 1 ||
		notification.instance.Publishers[0].Name != "audit" {
		t.Fatalf("publishers = %+v", notification.instance.Publishers)
	}
	if len(notification.instance.Subscriptions) != 1 ||
		notification.instance.Subscriptions[0].Publisher != "remote.applications" {
		t.Fatalf("subscriptions = %+v", notification.instance.Subscriptions)
	}
}

func TestEventRouterUsesABoundedNonblockingQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	router := &applicationEventRouter{
		ctx:   ctx,
		queue: make(chan applicationapi.Event, applicationEventQueueCapacity),
	}
	for index := 0; index < applicationEventQueueCapacity; index++ {
		router.enqueue(context.Background(), applicationapi.Event{Name: "queued"})
	}
	// A full queue drops the next event synchronously instead of waiting for a
	// worker or subscriber to make space.
	router.enqueue(context.Background(), applicationapi.Event{Name: "dropped"})
	if got := len(router.queue); got != applicationEventQueueCapacity {
		t.Fatalf("queue length = %d, want %d", got, applicationEventQueueCapacity)
	}
}

func TestEventRouterUnsubscribesWhenItsContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := newTestEventSource()
	newEventRoutingService(
		ctx,
		eventTestRegistry{},
		&fakeStore{},
		newRoutingHost(),
		source,
	)
	cancel()
	select {
	case <-source.unsubscribed:
	case <-time.After(time.Second):
		t.Fatal("event source was not unsubscribed on cancellation")
	}
}

func TestEventRouterCloseWaitsForActiveDeliveryAndUnsubscribe(t *testing.T) {
	ctx := context.Background()
	source := newTestEventSource()
	host := newRoutingHost()
	host.blocks = map[string]<-chan struct{}{"consumer-1": make(chan struct{})}
	application := subscriberApplication("consumer", "remote.applications", "started")
	store := &fakeStore{global: []Instance{
		runningApplicationInstance("consumer-1", "consumer"),
	}}
	service := newEventRoutingService(
		ctx,
		eventTestRegistry{"consumer": application},
		store,
		host,
		source,
	)

	source.publish(applicationapi.Event{
		Source: applicationapi.EventSource{
			ApplicationID: "remote", Publisher: "remote.applications",
		},
		Name: "started", Version: 1,
	})
	awaitNotification(t, host.notified)

	closed := make(chan struct{})
	go func() {
		service.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("service close did not cancel and join active event delivery")
	}
	select {
	case <-source.unsubscribed:
	case <-time.After(time.Second):
		t.Fatal("service close returned before the event router unsubscribed")
	}

	// Close remains safe after the worker is already gone.
	service.Close()
}

func newEventRoutingService(
	ctx context.Context,
	registry Registry,
	store Store,
	host BackendHost,
	source EventSource,
) *Service {
	return New(
		registry,
		store,
		nil,
		nil,
		nil,
		WithBackendHost(host),
		WithEventSource(ctx, source),
	)
}

func subscriberApplication(id, publisher, event string) Application {
	return subscriberApplicationWithTimeout(id, publisher, event, DefaultBackendTimeoutMS)
}

func subscriberApplicationWithTimeout(id, publisher, event string, timeoutMS int) Application {
	return Application{
		ID:      id,
		Name:    id,
		Version: "1",
		Scopes:  []Scope{ScopeGlobal, ScopeProject},
		Backend: &ApplicationBackend{TimeoutMS: timeoutMS},
		Subscriptions: []applicationapi.Subscription{{
			Publisher: publisher,
			Events:    []string{event},
		}},
	}
}

func runningApplicationInstance(id, applicationID string) Instance {
	return Instance{
		ID: id, ApplicationID: applicationID, Scope: ScopeGlobal, Status: StatusRunning,
	}
}

func runningProjectApplicationInstance(id, applicationID, projectID string) Instance {
	return Instance{
		ID: id, ApplicationID: applicationID, Scope: ScopeProject,
		ProjectID: projectID, Status: StatusRunning,
	}
}

func awaitNotification(t *testing.T, notifications <-chan routedNotification) routedNotification {
	t.Helper()
	select {
	case notification := <-notifications:
		return notification
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event delivery")
		return routedNotification{}
	}
}

func assertNoNotification(t *testing.T, notifications <-chan routedNotification) {
	t.Helper()
	select {
	case notification := <-notifications:
		t.Fatalf("unexpected event delivery to %s", notification.instance.ID)
	case <-time.After(50 * time.Millisecond):
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
