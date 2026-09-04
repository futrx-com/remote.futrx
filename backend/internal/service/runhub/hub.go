package runhub

import (
	"context"
	"log"
	"sync"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type EventStore interface {
	ReadEvents(ctx context.Context, chatID servicechat.ID) ([]servicechat.Event, error)
	ReadEventsAfter(ctx context.Context, chatID servicechat.ID, afterSeq int64) ([]servicechat.Event, error)
	AppendEvent(ctx context.Context, chatID servicechat.ID, ev servicechat.Event) (servicechat.Event, error)
}

type Hub struct {
	store             EventStore
	mu                sync.Mutex
	rooms             map[servicechat.ID]*room
	runningSubscriber func(servicechat.ID, bool)
}

type room struct {
	mu      sync.Mutex
	subs    map[*Subscription]struct{}
	running *runState
	nextRun uint64
}

type runState struct {
	id              uint64
	cancel          context.CancelFunc
	cancelRequested bool
}

const subscriptionLiveBuffer = 4096

type Subscription struct {
	room   *room
	events chan servicechat.Event
	mu     sync.Mutex
	closed bool
}

func New(store EventStore) *Hub {
	return &Hub{
		store: store,
		rooms: map[servicechat.ID]*room{},
	}
}

func (h *Hub) SetRunningSubscriber(fn func(servicechat.ID, bool)) {
	h.mu.Lock()
	h.runningSubscriber = fn
	h.mu.Unlock()
}

func (h *Hub) room(chatID servicechat.ID) *room {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.rooms[chatID]; ok {
		return r
	}
	r := &room{subs: map[*Subscription]struct{}{}}
	h.rooms[chatID] = r
	return r
}

func (h *Hub) Subscribe(ctx context.Context, chatID servicechat.ID) (*Subscription, error) {
	return h.SubscribeAfter(ctx, chatID, 0)
}

func (h *Hub) SubscribeAfter(
	ctx context.Context,
	chatID servicechat.ID,
	afterSeq int64,
) (*Subscription, error) {
	r := h.room(chatID)
	r.mu.Lock()
	defer r.mu.Unlock()

	var replay []servicechat.Event
	if h.store != nil {
		var events []servicechat.Event
		var err error
		if afterSeq > 0 {
			events, err = h.store.ReadEventsAfter(ctx, chatID, afterSeq)
		} else {
			events, err = h.store.ReadEvents(ctx, chatID)
		}
		if err != nil {
			return nil, err
		}
		replay = events
	}

	sub := &Subscription{
		room:   r,
		events: make(chan servicechat.Event, len(replay)+subscriptionLiveBuffer),
	}
	for _, ev := range replay {
		sub.events <- ev
	}
	sub.events <- servicechat.Event{
		T:       time.Now().UnixMilli(),
		Type:    "sync",
		Running: r.running != nil,
	}
	r.subs[sub] = struct{}{}
	return sub, nil
}

func (s *Subscription) Events() <-chan servicechat.Event {
	return s.events
}

func (s *Subscription) Close() {
	s.room.mu.Lock()
	delete(s.room.subs, s)
	s.room.mu.Unlock()
	s.closeChannel()
}

func (s *Subscription) SendTransient(ev servicechat.Event) {
	_ = s.trySend(ev)
}

func (s *Subscription) trySend(ev servicechat.Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.events <- ev:
		return true
	default:
		return false
	}
}

func (s *Subscription) closeChannel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.events)
}

func (h *Hub) Emit(chatID servicechat.ID, ev servicechat.Event) {
	stored := ev
	if h.store != nil {
		next, err := h.store.AppendEvent(context.Background(), chatID, ev)
		if err != nil {
			log.Printf("chat %s append: %v", chatID, err)
		} else {
			stored = next
		}
	}

	r := h.room(chatID)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.broadcastLocked(stored)
}

func (h *Hub) BroadcastTransient(chatID servicechat.ID, ev servicechat.Event) {
	r := h.room(chatID)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.broadcastLocked(ev)
}

func (r *room) broadcastLocked(ev servicechat.Event) {
	for sub := range r.subs {
		if sub.trySend(ev) {
			continue
		}
		delete(r.subs, sub)
		sub.closeChannel()
	}
}

func (h *Hub) StartRun(chatID servicechat.ID, cancel context.CancelFunc) (uint64, bool) {
	r := h.room(chatID)
	r.mu.Lock()
	if r.running != nil {
		r.mu.Unlock()
		return 0, false
	}
	r.nextRun++
	id := r.nextRun
	r.running = &runState{id: id, cancel: cancel}
	r.broadcastLocked(servicechat.Event{
		T:       time.Now().UnixMilli(),
		Type:    "sync",
		Running: true,
	})
	r.mu.Unlock()
	h.publishRunning(chatID, true)
	return id, true
}

func (h *Hub) FinishRun(chatID servicechat.ID, runID uint64) {
	r := h.room(chatID)
	r.mu.Lock()
	changed := false
	if r.running != nil && r.running.id == runID {
		r.running = nil
		r.broadcastLocked(servicechat.Event{
			T:    time.Now().UnixMilli(),
			Type: "sync",
		})
		changed = true
	}
	r.mu.Unlock()
	if changed {
		h.publishRunning(chatID, false)
	}
}

func (h *Hub) IsRunning(chatID servicechat.ID) bool {
	r := h.room(chatID)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running != nil
}

func (h *Hub) Cancel(ctx context.Context, chatID servicechat.ID) error {
	h.CancelRun(chatID)
	return nil
}

func (h *Hub) CancelRun(chatID servicechat.ID) bool {
	r := h.room(chatID)
	r.mu.Lock()
	running := r.running
	if running == nil {
		r.mu.Unlock()
		return false
	}
	if running.cancelRequested {
		r.mu.Unlock()
		return true
	}
	// Cancellation is only a request. Keep the run lock and running=true sync
	// state until the provider loop exits and FinishRun publishes the
	// authoritative terminal transition.
	running.cancelRequested = true
	r.mu.Unlock()

	running.cancel()
	return true
}

func (h *Hub) publishRunning(chatID servicechat.ID, running bool) {
	h.mu.Lock()
	fn := h.runningSubscriber
	h.mu.Unlock()
	if fn != nil {
		fn(chatID, running)
	}
}
