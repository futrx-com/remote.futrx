package rpc

import (
	"context"
	"fmt"
	"net/rpc"
	"sort"
	"sync"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	goplugin "github.com/hashicorp/go-plugin"
)

type server struct {
	impl     applications.Backend
	events   *runtimeEvents
	broker   *goplugin.MuxBroker
	requests requestCancellations
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
	requestContext, finish := s.requests.begin(args.RequestID)
	defer finish()
	request := args.Request.WithCancellation(requestContext)
	response, err := recovered(func() (applications.Response, error) {
		return s.impl.Handle(request)
	})
	content, size, modTime, streaming := response.ResponseStream()
	if err != nil {
		if streaming && content != nil {
			_ = closeWithoutPanic(content)
		}
		reply.Error = errorText(err)
		return nil
	}
	if streaming {
		switch {
		case content == nil:
			err = fmt.Errorf("stream content is nil")
		case size < 0:
			err = fmt.Errorf("stream size is negative")
		case response.Body != nil:
			err = fmt.Errorf("stream response also contains a buffered body")
		case response.Status != 0 && response.Status != 200:
			err = fmt.Errorf("stream response status must be 0 or 200")
		case s.broker == nil || !args.StreamBroker:
			err = fmt.Errorf("response stream broker is unavailable")
		}
		if err != nil {
			if content != nil {
				_ = closeWithoutPanic(content)
			}
			reply.Error = errorText(err)
			return nil
		}
		reply.Stream = &StreamInfo{Size: size, ModTime: modTime}
		go serveStream(s.broker, args.StreamBrokerID, content)
	}
	reply.Response = response
	return nil
}

// Cancel ends one in-flight Handle call. net/rpc reads requests in order but
// dispatches methods concurrently, so Cancel can run before Handle registers.
// requestCancellations retains that early signal and applies it at begin.
func (s *server) Cancel(args CancelArgs, reply *CancelReply) error {
	cause := error(context.Canceled)
	if args.DeadlineExceeded {
		cause = context.DeadlineExceeded
	}
	s.requests.cancel(args.RequestID, cause)
	reply.Acknowledged = true
	return nil
}

type requestCancellations struct {
	mu               sync.Mutex
	active           map[uint64]context.CancelCauseFunc
	pending          map[uint64]error
	completedThrough uint64
	completed        []requestIDRange
}

type requestIDRange struct {
	first uint64
	last  uint64
}

func (r *requestCancellations) begin(id uint64) (context.Context, func()) {
	if id == 0 {
		return context.Background(), func() {}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	r.mu.Lock()
	if r.active == nil {
		r.active = make(map[uint64]context.CancelCauseFunc)
	}
	r.active[id] = cancel
	if cause, ok := r.pending[id]; ok {
		delete(r.pending, id)
		cancel(cause)
	}
	r.mu.Unlock()
	return ctx, func() { r.finish(id, cancel) }
}

func (r *requestCancellations) cancel(id uint64, cause error) {
	if id == 0 {
		return
	}
	r.mu.Lock()
	if cancel := r.active[id]; cancel != nil {
		r.mu.Unlock()
		cancel(cause)
		return
	}
	if id <= r.completedThrough {
		r.mu.Unlock()
		return
	}
	completedIndex := sort.Search(len(r.completed), func(index int) bool {
		return r.completed[index].last >= id
	})
	if completedIndex < len(r.completed) && r.completed[completedIndex].first <= id {
		r.mu.Unlock()
		return
	}
	if r.pending == nil {
		r.pending = make(map[uint64]error)
	}
	r.pending[id] = cause
	r.mu.Unlock()
}

func (r *requestCancellations) finish(id uint64, cancel context.CancelCauseFunc) {
	cancel(context.Canceled)
	r.mu.Lock()
	delete(r.active, id)
	delete(r.pending, id)
	r.markCompleted(id)
	r.mu.Unlock()
}

// markCompleted records out-of-order completions as coalesced ranges. A slow
// low-numbered request must not make one registry entry accumulate for every
// later request that finishes. Gaps between ranges correspond to requests
// that are still active (or have not reached begin yet), so retained state is
// bounded by the amount of actual concurrency rather than request history.
// r.mu must be held by the caller.
func (r *requestCancellations) markCompleted(id uint64) {
	if id <= r.completedThrough {
		return
	}

	position := sort.Search(len(r.completed), func(index int) bool {
		return r.completed[index].first >= id
	})
	if position > 0 && r.completed[position-1].last >= id {
		return
	}
	if position < len(r.completed) && r.completed[position].first == id {
		return
	}

	joinsPrevious := position > 0 && consecutive(r.completed[position-1].last, id)
	joinsNext := position < len(r.completed) && consecutive(id, r.completed[position].first)
	switch {
	case joinsPrevious && joinsNext:
		r.completed[position-1].last = r.completed[position].last
		copy(r.completed[position:], r.completed[position+1:])
		r.completed = r.completed[:len(r.completed)-1]
	case joinsPrevious:
		r.completed[position-1].last = id
	case joinsNext:
		r.completed[position].first = id
	default:
		r.completed = append(r.completed, requestIDRange{})
		copy(r.completed[position+1:], r.completed[position:])
		r.completed[position] = requestIDRange{first: id, last: id}
	}

	for len(r.completed) > 0 && consecutive(r.completedThrough, r.completed[0].first) {
		r.completedThrough = r.completed[0].last
		copy(r.completed, r.completed[1:])
		r.completed = r.completed[:len(r.completed)-1]
	}
}

func consecutive(first uint64, second uint64) bool {
	return first < ^uint64(0) && first+1 == second
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
