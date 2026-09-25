package applications

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	applicationapi "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

const applicationEventQueueCapacity = 256

// applicationEventRouter separates event publication from backend delivery.
// The process event bus worker invokes enqueue inline, so this second bounded
// queue keeps backend startup and handlers from delaying that shared worker.
type applicationEventRouter struct {
	service *Service
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	close   sync.Once
	queue   chan applicationapi.Event
}

func (s *Service) startEventRouter() {
	if s.eventSource == nil {
		return
	}
	ctx := s.eventContext
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	router := &applicationEventRouter{
		service: s,
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
		queue:   make(chan applicationapi.Event, applicationEventQueueCapacity),
	}
	s.eventRouter = router
	unsubscribe := s.eventSource.Subscribe(router.enqueue)
	go router.run(unsubscribe)
}

// Close stops event routing and waits until its subscription and worker are
// gone. The process composition root calls this before shutting down backend
// children so an already accepted event cannot lazily launch a child after the
// host has taken its shutdown snapshot. It is safe to call more than once.
func (s *Service) Close() {
	if s.eventRouter != nil {
		s.eventRouter.Close()
	}
}

func (r *applicationEventRouter) Close() {
	r.close.Do(r.cancel)
	<-r.done
}

func (r *applicationEventRouter) enqueue(_ context.Context, event applicationapi.Event) {
	event = cloneRoutedEvent(event)
	select {
	case <-r.ctx.Done():
		return
	default:
	}
	select {
	case r.queue <- event:
	default:
		log.Printf(
			"applications: event queue full; dropping %s event %s",
			event.Source.Publisher,
			event.Name,
		)
	}
}

func (r *applicationEventRouter) run(unsubscribe func()) {
	defer close(r.done)
	if unsubscribe != nil {
		defer unsubscribe()
	}
	for {
		select {
		case <-r.ctx.Done():
			return
		case event := <-r.queue:
			r.route(event)
		}
	}
}

func (r *applicationEventRouter) route(event applicationapi.Event) {
	if r.service.backends == nil || !validEventOrigin(event.Source) {
		return
	}
	instances, err := r.service.store.ListAll(r.ctx)
	if err != nil {
		log.Printf(
			"applications: list event subscribers for %s event %s: %v",
			event.Source.Publisher,
			event.Name,
			err,
		)
		return
	}
	sort.Slice(instances, func(left, right int) bool {
		return instances[left].ID < instances[right].ID
	})
	for _, candidate := range instances {
		if r.ctx.Err() != nil {
			return
		}
		r.routeToInstance(event, candidate.ID)
	}
}

// routeToInstance reloads all eligibility under the same lock held through
// Notify. A ListAll result is only a candidate snapshot: Stop, Uninstall, or an
// upgrade may otherwise change the record and tear down its process before a
// stale routing decision lazily starts it again.
func (r *applicationEventRouter) routeToInstance(event applicationapi.Event, instanceID string) {
	unlock := r.service.instanceLocks.rlock(instanceID)
	defer unlock()

	if r.ctx.Err() != nil {
		return
	}
	instance, application, err := r.service.load(r.ctx, instanceID)
	if err != nil || instance.Status != StatusRunning ||
		!eventVisibleToInstance(event.Source, instance) ||
		application.Backend == nil || !applicationSubscribes(application, event) {
		return
	}

	ctx, cancel := eventDeliveryDeadline(r.ctx, *application.Backend)
	err = r.service.backends.Notify(
		ctx,
		backendInstanceDetails(application, instance),
		cloneRoutedEvent(event),
	)
	cancel()
	if err != nil {
		log.Printf(
			"applications: deliver %s event %s to instance %s: %v",
			event.Source.Publisher,
			event.Name,
			instance.ID,
			err,
		)
	}
}

// eventDeliveryDeadline keeps one slow subscriber from monopolizing the
// router's single ordered worker for the full duration of an unusually long
// browser-call timeout.
func eventDeliveryDeadline(
	ctx context.Context,
	backend ApplicationBackend,
) (context.Context, context.CancelFunc) {
	timeout := backend.Timeout()
	if timeout > MaxEventDeliveryTimeoutMS {
		timeout = MaxEventDeliveryTimeoutMS
	}
	return context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
}

func applicationSubscribes(application Application, event applicationapi.Event) bool {
	for _, subscription := range application.Subscriptions {
		if subscription.Publisher != event.Source.Publisher {
			continue
		}
		for _, name := range subscription.Events {
			if name == event.Name {
				return true
			}
		}
		return false
	}
	return false
}

func validEventOrigin(source applicationapi.EventSource) bool {
	switch source.Scope {
	case "", string(ScopeGlobal):
		return source.ProjectID == ""
	case string(ScopeProject):
		return source.ProjectID != ""
	default:
		return false
	}
}

func eventVisibleToInstance(source applicationapi.EventSource, instance Instance) bool {
	if source.Scope != string(ScopeProject) {
		return true
	}
	return instance.Scope == ScopeProject && instance.ProjectID == source.ProjectID
}

func cloneRoutedEvent(event applicationapi.Event) applicationapi.Event {
	event.Payload = append(event.Payload[:0:0], event.Payload...)
	return event
}
