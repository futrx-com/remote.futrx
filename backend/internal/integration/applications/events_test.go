package applications

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	applicationapi "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type recordingEventSink struct {
	mu     sync.Mutex
	events []applicationapi.Event
}

func (s *recordingEventSink) Publish(_ context.Context, event applicationapi.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	s.events = append(s.events, event)
}

func (s *recordingEventSink) last() (applicationapi.Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.events) == 0 {
		return applicationapi.Event{}, false
	}
	return s.events[len(s.events)-1], true
}

func publishingInstance() applicationapi.Instance {
	return applicationapi.Instance{
		ID:            "instance-1",
		ApplicationID: "hello-remote",
		Scope:         string(svc.ScopeProject),
		ProjectID:     "project-1",
		Publishers: []applicationapi.PublisherDeclaration{{
			Name: "greetings",
			Events: []applicationapi.EventDeclaration{{
				Name: "greeted", Version: 1,
			}},
		}},
	}
}

func TestInstancePublisherValidatesAndStampsPublication(t *testing.T) {
	sink := &recordingEventSink{}
	publisher := newInstancePublisher(publishingInstance(), sink)
	payload := json.RawMessage(`{"message":"hello"}`)

	if err := publisher.Emit(applicationapi.Publication{
		Publisher: "greetings",
		Event:     "greeted",
		Version:   1,
		Payload:   payload,
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	payload[0] = '['

	event, ok := sink.last()
	if !ok {
		t.Fatal("event was not sent to the bus")
	}
	if event.Source.ApplicationID != "hello-remote" ||
		event.Source.InstanceID != "instance-1" ||
		event.Source.Scope != string(svc.ScopeProject) ||
		event.Source.ProjectID != "project-1" ||
		event.Source.Publisher != "applications.hello-remote.greetings" {
		t.Fatalf("source = %+v", event.Source)
	}
	if event.Name != "greeted" || event.Version != 1 {
		t.Fatalf("event = %+v", event)
	}
	if string(event.Payload) != `{"message":"hello"}` {
		t.Fatalf("payload = %s", event.Payload)
	}
}

func TestInstancePublisherRejectsUndeclaredOrInvalidPublications(t *testing.T) {
	tooLarge := make(json.RawMessage, maxEventPayloadBytes+1)
	copy(tooLarge, `{"value":"`)
	for index := len(`{"value":"`); index < len(tooLarge)-2; index++ {
		tooLarge[index] = 'x'
	}
	copy(tooLarge[len(tooLarge)-2:], `"}`)

	tests := []struct {
		name        string
		publication applicationapi.Publication
		want        string
	}{
		{
			name:        "publisher",
			publication: applicationapi.Publication{Publisher: "other", Event: "greeted", Version: 1, Payload: json.RawMessage(`{}`)},
			want:        "not declared",
		},
		{
			name:        "event",
			publication: applicationapi.Publication{Publisher: "greetings", Event: "other", Version: 1, Payload: json.RawMessage(`{}`)},
			want:        "not declared",
		},
		{
			name:        "version",
			publication: applicationapi.Publication{Publisher: "greetings", Event: "greeted", Version: 2, Payload: json.RawMessage(`{}`)},
			want:        "not declared",
		},
		{
			name:        "malformed JSON",
			publication: applicationapi.Publication{Publisher: "greetings", Event: "greeted", Version: 1, Payload: json.RawMessage(`{"`)},
			want:        "valid JSON",
		},
		{
			name:        "array payload",
			publication: applicationapi.Publication{Publisher: "greetings", Event: "greeted", Version: 1, Payload: json.RawMessage(`[]`)},
			want:        "JSON object",
		},
		{
			name:        "null payload",
			publication: applicationapi.Publication{Publisher: "greetings", Event: "greeted", Version: 1, Payload: json.RawMessage(`null`)},
			want:        "JSON object",
		},
		{
			name:        "oversize payload",
			publication: applicationapi.Publication{Publisher: "greetings", Event: "greeted", Version: 1, Payload: tooLarge},
			want:        "exceeds 65536 bytes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink := &recordingEventSink{}
			err := newInstancePublisher(publishingInstance(), sink).Emit(test.publication)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if _, ok := sink.last(); ok {
				t.Fatal("invalid publication reached the event bus")
			}
		})
	}
}

func TestInstancePublisherUsesEmptyObjectForAnOmittedPayload(t *testing.T) {
	sink := &recordingEventSink{}
	err := newInstancePublisher(publishingInstance(), sink).Emit(applicationapi.Publication{
		Publisher: "greetings",
		Event:     "greeted",
		Version:   1,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	event, _ := sink.last()
	if string(event.Payload) != `{}` {
		t.Fatalf("payload = %s, want {}", event.Payload)
	}
}

const eventBackendSource = `package main

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

type backend struct {
	emitter applications.EventEmitter
	mu sync.Mutex
	received []applications.Event
}

func (b *backend) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{APIVersion: applications.APIVersion}, nil
}

func (b *backend) Init(applications.Instance) error {
	return b.emitter.Emit(applications.Publication{
		Publisher: "greetings", Event: "greeted", Version: 1,
		Payload: json.RawMessage("{\"stage\":\"init\"}"),
	})
}

func (b *backend) Handle(request applications.Request) (applications.Response, error) {
	switch request.Path {
	case "publish":
		err := b.emitter.Emit(applications.Publication{
			Publisher: "greetings", Event: "greeted", Version: 1,
			Payload: json.RawMessage("{\"message\":\"hello\"}"),
		})
		if err != nil { return applications.Errorf(http.StatusBadRequest, "%v", err), nil }
		return applications.Text(http.StatusNoContent, ""), nil
	case "received":
		b.mu.Lock()
		defer b.mu.Unlock()
		return applications.JSON(http.StatusOK, b.received), nil
	case "pid":
		return applications.JSON(http.StatusOK, map[string]any{"pid": os.Getpid()}), nil
	}
	return applications.Errorf(http.StatusNotFound, "not found"), nil
}

func (b *backend) OnEvent(event applications.Event) error {
	if event.Name == "hang" { select {} }
	b.mu.Lock()
	defer b.mu.Unlock()
	b.received = append(b.received, event)
	return nil
}

func main() {
	rpc.ServeWithRuntime(func(runtime applications.Runtime) applications.Backend {
		return &backend{emitter: runtime.Events}
	})
}
`

func TestHostCarriesEventsBothWaysAcrossTheBackendProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a backend with the Go toolchain")
	}
	sink := &recordingEventSink{}
	host := New(
		sharedRoot(t),
		fakeCatalog{"event-application": sourceFS(eventBackendSource)},
		Options{GoTool: testGoToolOverride(), Events: sink},
	)
	t.Cleanup(host.Shutdown)
	spec := testInstance("event-application", "event-instance")
	spec.Publishers = publishingInstance().Publishers
	spec.Subscriptions = []applicationapi.Subscription{{
		Publisher: "remote.applications",
		Events:    []string{"started"},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	descriptor, err := host.Ensure(ctx, spec)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !descriptor.PublishesEvents || !descriptor.SubscribesEvents {
		t.Fatalf("capabilities = %+v", descriptor)
	}
	initialized, ok := sink.last()
	if !ok || string(initialized.Payload) != `{"stage":"init"}` {
		t.Fatalf("event runtime was not bound before Backend.Init: %+v", initialized)
	}

	response, err := host.Call(ctx, spec, applicationapi.Request{Path: "publish"})
	if err != nil || response.Status != httpStatusNoContent {
		t.Fatalf("publish response = %+v, error = %v", response, err)
	}
	published, ok := sink.last()
	if !ok || published.Source.Publisher != "applications.event-application.greetings" {
		t.Fatalf("published event = %+v", published)
	}

	delivered := applicationapi.Event{
		Source: applicationapi.EventSource{ApplicationID: "remote", Publisher: "remote.applications"},
		Name:   "started", Version: 1, Payload: json.RawMessage(`{"applicationId":"hello-remote"}`),
	}
	if err := host.Notify(ctx, spec, delivered); err != nil {
		t.Fatalf("notify: %v", err)
	}
	response, err = host.Call(ctx, spec, applicationapi.Request{Path: "received"})
	if err != nil || !strings.Contains(string(response.Body), `"name":"started"`) {
		t.Fatalf("received response = %s, error = %v", response.Body, err)
	}
}

func TestHostRejectsDeclaredPublishersWithoutRuntimeEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a backend with the Go toolchain")
	}
	host := New(
		sharedRoot(t),
		fakeCatalog{"missing-event-runtime": sourceFS(testBackendSource)},
		Options{GoTool: testGoToolOverride(), Events: &recordingEventSink{}},
	)
	t.Cleanup(host.Shutdown)
	spec := testInstance("missing-event-runtime", "missing-event-runtime-instance")
	spec.Publishers = publishingInstance().Publishers

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_, err := host.Ensure(ctx, spec)
	if err == nil || !strings.Contains(err.Error(), "did not request application runtime events") {
		t.Fatalf("ensure error = %v, want missing runtime rejection", err)
	}
}

func TestTimedOutEventKillsAndLazilyReinitializesBackend(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a backend with the Go toolchain")
	}
	sink := &recordingEventSink{}
	host := New(
		sharedRoot(t),
		fakeCatalog{"event-timeout-application": sourceFS(eventBackendSource)},
		Options{GoTool: testGoToolOverride(), Events: sink},
	)
	t.Cleanup(host.Shutdown)
	spec := testInstance("event-timeout-application", "event-timeout-instance")
	spec.Publishers = publishingInstance().Publishers
	spec.Subscriptions = []applicationapi.Subscription{{
		Publisher: "remote.applications",
		Events:    []string{"hang"},
	}}

	before := backendPID(t, call(t, host, spec, applicationapi.Request{Path: "pid"}))
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	err := host.Notify(ctx, spec, applicationapi.Event{
		Source: applicationapi.EventSource{ApplicationID: "remote", Publisher: "remote.applications"},
		Name:   "hang", Version: 1, Payload: json.RawMessage(`{}`),
	})
	cancel()
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("notify error = %v, want timeout", err)
	}

	after := backendPID(t, call(t, host, spec, applicationapi.Request{Path: "pid"}))
	if after == before {
		t.Fatalf("timed-out backend reused pid %d", after)
	}
	response := call(t, host, spec, applicationapi.Request{Path: "publish"})
	if response.Status != httpStatusNoContent {
		t.Fatalf("publish after restart = %+v", response)
	}
	if event, ok := sink.last(); !ok || event.Source.Publisher != "applications.event-timeout-application.greetings" {
		t.Fatalf("publisher was not reinitialized after restart: %+v", event)
	}
}

const httpStatusNoContent = 204
