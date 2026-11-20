package runhub

import (
	"context"
	"errors"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type failingEventStore struct {
	err     error
	appends int
}

func (s *failingEventStore) ReadEvents(context.Context, servicechat.ID) ([]servicechat.Event, error) {
	return nil, nil
}
func (s *failingEventStore) ReadEventsAfter(context.Context, servicechat.ID, int64) ([]servicechat.Event, error) {
	return nil, nil
}
func (s *failingEventStore) AppendEvent(_ context.Context, _ servicechat.ID, event servicechat.Event) (servicechat.Event, error) {
	s.appends++
	return event, s.err
}

func TestPersistenceFailureStopsRunAndDoesNotBroadcastUncommittedOutput(t *testing.T) {
	store := &failingEventStore{err: errors.New("disk full")}
	hub := New(store)
	sub, err := hub.Subscribe(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	<-sub.Events()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id, ok := hub.StartRun("abcd", cancel)
	if !ok {
		t.Fatal("start failed")
	}
	<-sub.Events()
	hub.Emit("abcd", servicechat.Event{Type: "assistant_text", Text: "not committed"})
	if !errors.Is(hub.PersistenceError("abcd"), store.err) {
		t.Fatal("persistence failure lost")
	}
	if ctx.Err() == nil {
		t.Fatal("run was not cancelled")
	}
	if event := <-sub.Events(); event.Type != "error" || event.Seq != 0 {
		t.Fatalf("unexpected event: %#v", event)
	}
	hub.Emit("abcd", servicechat.Event{Type: "complete"})
	if store.appends != 1 {
		t.Fatal("events continued after persistence failed")
	}
	select {
	case event := <-sub.Events():
		t.Fatalf("uncommitted event broadcast: %#v", event)
	default:
	}
	hub.FinishRun("abcd", id)
	store.err = nil
	if _, ok := hub.StartRun("abcd", func() {}); !ok {
		t.Fatal("retry failed")
	}
	if hub.PersistenceError("abcd") != nil {
		t.Fatal("retry retained previous failure")
	}
}
