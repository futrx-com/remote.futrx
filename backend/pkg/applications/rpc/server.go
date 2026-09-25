package rpc

import (
	"fmt"
	"net/rpc"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	goplugin "github.com/hashicorp/go-plugin"
)

type server struct {
	impl   applications.Backend
	events *runtimeEvents
	broker *goplugin.MuxBroker
}

func (s *server) Describe(_ DescribeArgs, reply *DescribeReply) error {
	descriptor, err := recovered(func() (applications.Descriptor, error) {
		return s.impl.Describe()
	})
	// Publishing is manifest-owned and the host fills it after this handshake.
	// Subscription remains an optional backend implementation capability. Never
	// trust mutable values a backend happened to return from Describe.
	descriptor.PublishesEvents = false
	_, descriptor.SubscribesEvents = s.impl.(applications.EventSubscriber)
	reply.Descriptor = descriptor
	reply.Error = errorText(err)
	return nil
}

func (s *server) Init(args InitArgs, reply *InitReply) error {
	_, err := recovered(func() (struct{}, error) {
		return struct{}{}, s.impl.Init(args.Instance)
	})
	reply.Error = errorText(err)
	return nil
}

func (s *server) Handle(args HandleArgs, reply *HandleReply) error {
	response, err := recovered(func() (applications.Response, error) {
		return s.impl.Handle(args.Request)
	})
	reply.Response = response
	reply.Error = errorText(err)
	return nil
}

func (s *server) BindEvents(args BindEventsArgs, reply *BindEventsReply) error {
	if s.events == nil {
		reply.Error = "backend did not request application runtime events"
		return nil
	}
	if s.broker == nil {
		reply.Error = "event callback broker is unavailable"
		return nil
	}
	connection, err := s.broker.Dial(args.BrokerID)
	if err != nil {
		reply.Error = fmt.Sprintf("connect event callback: %v", err)
		return nil
	}
	emitter := &eventEmitterClient{client: rpc.NewClient(connection)}
	_, err = recovered(func() (struct{}, error) {
		return struct{}{}, s.events.bind(emitter)
	})
	if err != nil {
		_ = emitter.client.Close()
	}
	reply.Error = errorText(err)
	return nil
}

func (s *server) OnEvent(args OnEventArgs, reply *OnEventReply) error {
	backend, ok := s.impl.(applications.EventSubscriber)
	if !ok {
		reply.Error = "backend does not implement applications.EventSubscriber"
		return nil
	}
	_, err := recovered(func() (struct{}, error) {
		return struct{}{}, backend.OnEvent(args.Event)
	})
	reply.Error = errorText(err)
	return nil
}

// eventEmitterServer exposes the host-owned emitter on the callback connection
// initiated while core binds the application runtime.
type eventEmitterServer struct {
	impl applications.EventEmitter
}

func (s *eventEmitterServer) Emit(args EmitArgs, reply *EmitReply) error {
	_, err := recovered(func() (struct{}, error) {
		return struct{}{}, s.impl.Emit(args.Publication)
	})
	reply.Error = errorText(err)
	return nil
}

// recovered turns a panicking backend method into an ordinary error. A backend
// that panics costs its caller one failed request, not the process and every
// other request in flight on it.
func recovered[T any](call func() (T, error)) (result T, err error) {
	defer func() {
		if recovery := recover(); recovery != nil {
			var zero T
			result = zero
			err = fmt.Errorf("backend panicked: %v", recovery)
		}
	}()
	return call()
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
