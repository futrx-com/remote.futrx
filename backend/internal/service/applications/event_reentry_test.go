package applications

import (
	"context"
	"sync"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/lifecycle"
	applicationapi "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// reentryHost models the important process boundary: an application may
// publish while Remote is holding the instance read lock for Handle or
// OnEvent. A direct core event hook is then allowed to request a lifecycle
// write without forming a lock cycle with that publication.
type reentryHost struct {
	bus         *lifecycle.EventBus
	callEvent   *applicationapi.Event
	notifyEvent *applicationapi.Event
	notified    chan struct{}
	stopped     chan struct{}
	notifyOnce  sync.Once
	stopOnce    sync.Once
}

func (*reentryHost) Ensure(
	context.Context,
	applicationapi.Instance,
) (applicationapi.Descriptor, error) {
	return applicationapi.Descriptor{APIVersion: applicationapi.APIVersion}, nil
}

func (h *reentryHost) Call(
	ctx context.Context,
	_ applicationapi.Instance,
	_ applicationapi.Request,
) (applicationapi.Response, error) {
	if h.callEvent != nil {
		h.bus.Publish(ctx, *h.callEvent)
	}
	return applicationapi.Response{Status: 200}, nil
}

func (h *reentryHost) Notify(
	ctx context.Context,
	_ applicationapi.Instance,
	_ applicationapi.Event,
) error {
	if h.notifyEvent != nil {
		h.bus.Publish(ctx, *h.notifyEvent)
	}
	h.notifyOnce.Do(func() { close(h.notified) })
	return nil
}

func (h *reentryHost) Stop(context.Context, string) error {
	h.stopOnce.Do(func() { close(h.stopped) })
	return nil
}

func (*reentryHost) Remove(context.Context, string) error { return nil }
func (*reentryHost) InvalidateApplication(string)         {}

func TestBackendCallPublicationAllowsCoreLifecycleReentry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	bus := lifecycle.NewEventBus(ctx)
	t.Cleanup(func() {
		cancel()
		bus.Close()
	})
	host := &reentryHost{
		bus:       bus,
		callEvent: reentryEvent("from-handle"),
		notified:  make(chan struct{}),
		stopped:   make(chan struct{}),
	}
	application := subscriberApplication("consumer", "remote.applications", "installed")
	store := &fakeStore{global: []Instance{runningApplicationInstance("copy-1", "consumer")}}
	service := New(
		eventTestRegistry{"consumer": application},
		store,
		nil,
		nil,
		nil,
		WithBackendHost(host),
	)

	stopped := make(chan error, 1)
	bus.Subscribe(func(_ context.Context, event applicationapi.Event) {
		if event.Name != "from-handle" {
			return
		}
		_, err := service.Stop(context.Background(), "copy-1")
		stopped <- err
	})

	called := make(chan error, 1)
	go func() {
		_, err := service.CallBackend(
			context.Background(),
			"copy-1",
			applicationapi.Request{Path: "publish"},
			applicationapi.Caller{Email: "user@example.com"},
		)
		called <- err
	}()
	if err := awaitError(t, called, "backend publication to return"); err != nil {
		t.Fatalf("call backend: %v", err)
	}
	if err := awaitError(t, stopped, "core lifecycle reentry after Handle"); err != nil {
		t.Fatalf("stop from core hook: %v", err)
	}
	awaitSignal(t, host.stopped, "backend stop")
}

func TestBackendEventPublicationAllowsCoreLifecycleReentry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	bus := lifecycle.NewEventBus(ctx)
	t.Cleanup(func() {
		cancel()
		bus.Close()
	})
	host := &reentryHost{
		bus:         bus,
		notifyEvent: reentryEvent("from-on-event"),
		notified:    make(chan struct{}),
		stopped:     make(chan struct{}),
	}
	application := subscriberApplication(
		"consumer",
		"applications.producer.activity",
		"trigger",
	)
	store := &fakeStore{global: []Instance{runningApplicationInstance("copy-1", "consumer")}}
	service := New(
		eventTestRegistry{"consumer": application},
		store,
		nil,
		nil,
		nil,
		WithBackendHost(host),
		WithEventSource(ctx, bus),
	)

	stopped := make(chan error, 1)
	bus.Subscribe(func(_ context.Context, event applicationapi.Event) {
		if event.Name != "from-on-event" {
			return
		}
		_, err := service.Stop(context.Background(), "copy-1")
		stopped <- err
	})
	bus.Publish(context.Background(), applicationapi.Event{
		Source: applicationapi.EventSource{
			ApplicationID: "producer",
			InstanceID:    "producer-1",
			Scope:         string(ScopeGlobal),
			Publisher:     "applications.producer.activity",
		},
		Name: "trigger", Version: 1,
	})

	awaitSignal(t, host.notified, "OnEvent publication to return")
	if err := awaitError(t, stopped, "core lifecycle reentry after OnEvent"); err != nil {
		t.Fatalf("stop from core hook: %v", err)
	}
	awaitSignal(t, host.stopped, "backend stop")
}

func reentryEvent(name string) *applicationapi.Event {
	return &applicationapi.Event{
		Source: applicationapi.EventSource{
			ApplicationID: "consumer",
			InstanceID:    "copy-1",
			Scope:         string(ScopeGlobal),
			Publisher:     "applications.consumer.events",
		},
		Name: name, Version: 1,
	}
}
