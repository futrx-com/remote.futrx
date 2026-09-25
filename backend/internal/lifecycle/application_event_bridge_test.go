package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type recordingApplicationEventSink struct {
	contexts []context.Context
	events   []applications.Event
}

func (s *recordingApplicationEventSink) Publish(ctx context.Context, event applications.Event) {
	s.contexts = append(s.contexts, ctx)
	s.events = append(s.events, event)
}

func TestApplicationEventBridgePublishesCanonicalCatalogEvents(t *testing.T) {
	tests := []struct {
		state ApplicationCatalogState
		name  string
	}{
		{ApplicationAdded, applications.RemoteApplicationAdded},
		{ApplicationUpdated, applications.RemoteApplicationUpdated},
		{ApplicationDeleted, applications.RemoteApplicationDeleted},
	}
	for _, test := range tests {
		t.Run(string(test.state), func(t *testing.T) {
			sink := &recordingApplicationEventSink{}
			bridge := NewApplicationEventBridge(sink)
			ctx := context.Background()

			bridge.OnApplicationCatalog(ctx, ApplicationCatalogEvent{
				State: test.state, ApplicationID: "hello-remote",
			})

			if len(sink.events) != 1 {
				t.Fatalf("events = %d, want 1", len(sink.events))
			}
			got := sink.events[0]
			wantSource := applications.EventSource{
				ApplicationID: coreApplicationSource,
				Publisher:     CoreApplicationEventPublisher,
			}
			if got.Source != wantSource {
				t.Fatalf("source = %+v, want %+v", got.Source, wantSource)
			}
			if got.Name != test.name || got.Version != applications.RemoteApplicationsVersion {
				t.Fatalf("identity = %q v%d, want %q v%d",
					got.Name, got.Version, test.name, applications.RemoteApplicationsVersion)
			}
			assertCoreApplicationPayload(t, got.Payload, coreApplicationPayload{
				Version: CoreApplicationEventVersion, ApplicationID: "hello-remote",
			})
			if sink.contexts[0] != ctx {
				t.Fatal("bridge did not forward the lifecycle context")
			}
		})
	}
}

func TestApplicationEventBridgePublishesCanonicalInstanceEvents(t *testing.T) {
	tests := []struct {
		state ApplicationInstanceState
		name  string
	}{
		{ApplicationInstalled, applications.RemoteApplicationInstalled},
		{ApplicationUninstalled, applications.RemoteApplicationUninstalled},
		{ApplicationStarted, applications.RemoteApplicationStarted},
		{ApplicationStopped, applications.RemoteApplicationStopped},
	}
	for _, test := range tests {
		t.Run(string(test.state), func(t *testing.T) {
			sink := &recordingApplicationEventSink{}
			bridge := NewApplicationEventBridge(sink)

			bridge.OnApplicationInstance(context.Background(), ApplicationInstanceEvent{
				State:         test.state,
				ApplicationID: "hello-remote",
				InstanceID:    "instance-1",
				Scope:         "project",
				ProjectID:     "project-1",
			})

			if len(sink.events) != 1 {
				t.Fatalf("events = %d, want 1", len(sink.events))
			}
			got := sink.events[0]
			wantSource := applications.EventSource{
				ApplicationID: coreApplicationSource,
				InstanceID:    "instance-1",
				Scope:         "project",
				ProjectID:     "project-1",
				Publisher:     CoreApplicationEventPublisher,
			}
			if got.Source != wantSource {
				t.Fatalf("source = %+v, want %+v", got.Source, wantSource)
			}
			if got.Name != test.name || got.Version != applications.RemoteApplicationsVersion {
				t.Fatalf("identity = %q v%d, want %q v%d",
					got.Name, got.Version, test.name, applications.RemoteApplicationsVersion)
			}
			assertCoreApplicationPayload(t, got.Payload, coreApplicationPayload{
				Version:       CoreApplicationEventVersion,
				ApplicationID: "hello-remote",
				InstanceID:    "instance-1",
				Scope:         "project",
				ProjectID:     "project-1",
			})
		})
	}
}

func TestApplicationEventBridgeOmitsEmptyProjectFromGlobalPayload(t *testing.T) {
	sink := &recordingApplicationEventSink{}
	bridge := NewApplicationEventBridge(sink)
	bridge.OnApplicationInstance(context.Background(), ApplicationInstanceEvent{
		State: ApplicationInstalled, ApplicationID: "hello-remote",
		InstanceID: "global-1", Scope: "global",
	})

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(sink.events[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if _, exists := payload["projectId"]; exists {
		t.Fatalf("global payload contains projectId: %s", sink.events[0].Payload)
	}
	if sink.events[0].Source.ProjectID != "" {
		t.Fatalf("global source project = %q, want empty", sink.events[0].Source.ProjectID)
	}
}

func TestApplicationEventBridgeIgnoresUnknownLifecycleStates(t *testing.T) {
	sink := &recordingApplicationEventSink{}
	bridge := NewApplicationEventBridge(sink)

	bridge.OnApplicationCatalog(context.Background(), ApplicationCatalogEvent{
		State: "unknown", ApplicationID: "demo",
	})
	bridge.OnApplicationInstance(context.Background(), ApplicationInstanceEvent{
		State: "unknown", ApplicationID: "demo", InstanceID: "one",
	})

	if len(sink.events) != 0 {
		t.Fatalf("unknown states published events: %+v", sink.events)
	}
}

func TestApplicationEventBridgeConnectsTypedPublisherToDynamicBus(t *testing.T) {
	bus := NewEventBus(context.Background())
	t.Cleanup(bus.Close)
	bridge := NewApplicationEventBridge(bus)
	publisher := NewApplicationPublisher()
	publisher.SubscribeCatalog(bridge)
	publisher.SubscribeInstances(bridge)
	names := make(chan string, 2)
	bus.Subscribe(func(_ context.Context, event applications.Event) {
		names <- event.Name
	})

	publisher.PublishApplicationAdded(context.Background(), "demo")
	publisher.PublishApplicationInstalled(context.Background(), "demo", "one", "global", "")

	for _, want := range []string{"added", "installed"} {
		if got := awaitEventBusValue(t, names, "bridged application event"); got != want {
			t.Fatalf("dynamic event = %q, want %q", got, want)
		}
	}
}

func assertCoreApplicationPayload(t *testing.T, raw json.RawMessage, want coreApplicationPayload) {
	t.Helper()
	var got coreApplicationPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode payload %q: %v", raw, err)
	}
	if got != want {
		t.Fatalf("payload = %+v, want %+v", got, want)
	}
}
