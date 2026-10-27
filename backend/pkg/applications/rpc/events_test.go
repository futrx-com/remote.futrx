package rpc

import (
	"errors"
	"net"
	"net/rpc"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type requiredBackend struct {
	descriptor applications.Descriptor
	instance   applications.Instance
}

func (b *requiredBackend) Describe() (applications.Descriptor, error) {
	return b.descriptor, nil
}

func (b *requiredBackend) Init(instance applications.Instance) error {
	b.instance = instance
	return nil
}

func (*requiredBackend) Handle(applications.Request) (applications.Response, error) {
	return applications.Response{}, nil
}

type eventBackend struct {
	requiredBackend
	events []applications.Event
	err    error
	panic  bool
}

func (b *eventBackend) OnEvent(event applications.Event) error {
	if b.panic {
		panic("event failure")
	}
	b.events = append(b.events, event)
	return b.err
}

func TestDescribeDerivesEventCapabilities(t *testing.T) {
	for _, test := range []struct {
		name             string
		backend          applications.Backend
		publishesEvents  bool
		subscribesEvents bool
	}{
		{
			name: "required backend cannot claim optional capabilities",
			backend: &requiredBackend{descriptor: applications.Descriptor{
				PublishesEvents: true, SubscribesEvents: true,
			}},
		},
		{
			name:             "subscriber interface is discovered",
			backend:          &eventBackend{},
			publishesEvents:  false,
			subscribesEvents: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var reply DescribeReply
			if err := (&server{impl: test.backend}).Describe(DescribeArgs{}, &reply); err != nil {
				t.Fatalf("Describe() transport error: %v", err)
			}
			if reply.Error != "" {
				t.Fatalf("Describe() backend error: %s", reply.Error)
			}
			if reply.Descriptor.PublishesEvents != test.publishesEvents ||
				reply.Descriptor.SubscribesEvents != test.subscribesEvents {
				t.Fatalf(
					"capabilities = publish %t, subscribe %t; want %t, %t",
					reply.Descriptor.PublishesEvents,
					reply.Descriptor.SubscribesEvents,
					test.publishesEvents,
					test.subscribesEvents,
				)
			}
		})
	}
}

func TestEventSubscriberCrossesPrimaryRPCConnection(t *testing.T) {
	backend := &eventBackend{}
	client, closeClient := rpcClientFor(t, &server{impl: backend})
	defer closeClient()

	event := applications.Event{
		Source: applications.EventSource{
			ApplicationID: "remote",
			Publisher:     "remote.applications",
		},
		Name:    "installed",
		Version: 1,
		Payload: []byte(`{"applicationId":"hello-remote"}`),
	}
	if err := (&Client{client: client}).OnEvent(event); err != nil {
		t.Fatalf("OnEvent() error: %v", err)
	}
	if !reflect.DeepEqual(backend.events, []applications.Event{event}) {
		t.Fatalf("delivered events = %#v, want %#v", backend.events, []applications.Event{event})
	}
}

func TestInitCarriesEventDeclarationsAcrossPrimaryRPCConnection(t *testing.T) {
	backend := &requiredBackend{}
	client, closeClient := rpcClientFor(t, &server{impl: backend})
	defer closeClient()

	instance := applications.Instance{
		ID:            "instance-1",
		ApplicationID: "hello-remote",
		Publishers: []applications.PublisherDeclaration{{
			Name: "greetings",
			Events: []applications.EventDeclaration{{
				Name: "greeted", Version: 1, Description: "A greeting was recorded.",
			}},
		}},
		Subscriptions: []applications.Subscription{{
			Publisher: "remote.applications",
			Events:    []string{"installed", "stopped"},
		}},
	}
	if err := (&Client{client: client}).Init(instance); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if !reflect.DeepEqual(backend.instance, instance) {
		t.Fatalf("initialized instance = %#v, want %#v", backend.instance, instance)
	}
}

func TestEventSubscriberErrorsAndPanicsStayRPCFailures(t *testing.T) {
	for _, test := range []struct {
		name    string
		backend *eventBackend
		want    string
	}{
		{name: "error", backend: &eventBackend{err: errors.New("rejected")}, want: "rejected"},
		{name: "panic", backend: &eventBackend{panic: true}, want: "backend panicked: event failure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, closeClient := rpcClientFor(t, &server{impl: test.backend})
			defer closeClient()
			err := (&Client{client: client}).OnEvent(applications.Event{Name: "test", Version: 1})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("OnEvent() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

type recordingEmitter struct {
	publications []applications.Publication
	err          error
}

func (e *recordingEmitter) Emit(publication applications.Publication) error {
	e.publications = append(e.publications, publication)
	return e.err
}

func TestEventEmitterCrossesCallbackRPCConnection(t *testing.T) {
	emitter := &recordingEmitter{}
	client, closeClient := rpcClientFor(t, &eventEmitterServer{impl: emitter})
	defer closeClient()

	publication := applications.Publication{
		Publisher: "greetings",
		Event:     "greeted",
		Version:   1,
		Payload:   []byte(`{"count":2}`),
	}
	if err := (&eventEmitterClient{client: client}).Emit(publication); err != nil {
		t.Fatalf("Emit() error: %v", err)
	}
	if !reflect.DeepEqual(emitter.publications, []applications.Publication{publication}) {
		t.Fatalf("publications = %#v, want %#v", emitter.publications, []applications.Publication{publication})
	}
}

func TestEventEmitterRejectsOversizedPayloadBeforeCallbackRPC(t *testing.T) {
	emitter := &recordingEmitter{}
	client, closeClient := rpcClientFor(t, &eventEmitterServer{impl: emitter})
	defer closeClient()

	err := (&eventEmitterClient{client: client}).Emit(applications.Publication{
		Publisher: "greetings",
		Event:     "greeted",
		Version:   1,
		Payload:   make([]byte, applications.MaxEventPayloadBytes+1),
	})
	if err == nil || !strings.Contains(err.Error(), "payload exceeds 65536 bytes") {
		t.Fatalf("Emit() error = %v, want payload size rejection", err)
	}
	if len(emitter.publications) != 0 {
		t.Fatalf("callback received %d publications, want none", len(emitter.publications))
	}
}

func TestRuntimeEventsForwardsOnlyAfterCoreBindsIt(t *testing.T) {
	events := &runtimeEvents{}
	publication := applications.Publication{Publisher: "greetings", Event: "greeted", Version: 1}
	if err := events.Emit(publication); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("Emit() before bind error = %v", err)
	}
	emitter := &recordingEmitter{}
	if err := events.bind(emitter); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := events.Emit(publication); err != nil {
		t.Fatalf("Emit() after bind: %v", err)
	}
	if !reflect.DeepEqual(emitter.publications, []applications.Publication{publication}) {
		t.Fatalf("publications = %#v", emitter.publications)
	}
}

type recordingBroker struct {
	served chan brokeredServer
}

type brokeredServer struct {
	id     uint32
	server any
}

func (*recordingBroker) NextId() uint32 { return 41 }

func (b *recordingBroker) AcceptAndServe(id uint32, server any) {
	b.served <- brokeredServer{id: id, server: server}
}

type bindEventsRPC struct {
	mu   sync.Mutex
	args []BindEventsArgs
}

func (s *bindEventsRPC) BindEvents(args BindEventsArgs, _ *BindEventsReply) error {
	s.mu.Lock()
	s.args = append(s.args, args)
	s.mu.Unlock()
	return nil
}

func TestBindEventsOffersCoreEmitterThroughBroker(t *testing.T) {
	remote := &bindEventsRPC{}
	client, closeClient := rpcClientFor(t, remote)
	defer closeClient()
	broker := &recordingBroker{served: make(chan brokeredServer, 1)}

	if err := (&Client{client: client, broker: broker}).BindEvents(&recordingEmitter{}); err != nil {
		t.Fatalf("BindEvents() error: %v", err)
	}
	remote.mu.Lock()
	args := append([]BindEventsArgs(nil), remote.args...)
	remote.mu.Unlock()
	if !reflect.DeepEqual(args, []BindEventsArgs{{BrokerID: 41}}) {
		t.Fatalf("BindEvents args = %#v", args)
	}
	select {
	case offered := <-broker.served:
		if offered.id != 41 {
			t.Fatalf("broker id = %d, want 41", offered.id)
		}
		if _, ok := offered.server.(*eventEmitterServer); !ok {
			t.Fatalf("broker server = %T, want *eventEmitterServer", offered.server)
		}
	case <-time.After(time.Second):
		t.Fatal("event callback was not offered through the broker")
	}
}

func rpcClientFor(t *testing.T, service any) (*rpc.Client, func()) {
	t.Helper()
	serverConnection, clientConnection := net.Pipe()
	server := rpc.NewServer()
	if err := server.RegisterName("Plugin", service); err != nil {
		t.Fatalf("register RPC service: %v", err)
	}
	go server.ServeConn(serverConnection)
	client := rpc.NewClient(clientConnection)
	return client, func() {
		_ = client.Close()
		_ = serverConnection.Close()
	}
}
