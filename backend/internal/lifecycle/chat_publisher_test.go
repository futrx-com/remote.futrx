package lifecycle

import (
	"context"
	"testing"
)

type recordingChatSubscriber struct {
	ctx    context.Context
	events []ChatEvent
}

func (s *recordingChatSubscriber) OnChat(ctx context.Context, event ChatEvent) {
	s.ctx = ctx
	s.events = append(s.events, event)
}

func TestChatPublisherConstructsChatEvents(t *testing.T) {
	publisher := NewChatPublisher()
	subscriber := &recordingChatSubscriber{}
	publisher.Subscribe(subscriber)
	ctx := context.Background()

	publisher.PublishChatCreated(ctx, "deadbeef")
	publisher.PublishChatUpdated(ctx, "cafebabe")
	publisher.PublishChatDeleted(ctx, "0123abcd")

	want := []ChatEvent{
		{State: ChatCreated, ChatID: "deadbeef"},
		{State: ChatUpdated, ChatID: "cafebabe"},
		{State: ChatDeleted, ChatID: "0123abcd"},
	}
	if len(subscriber.events) != len(want) {
		t.Fatalf("events = %d, want %d", len(subscriber.events), len(want))
	}
	for index := range want {
		if subscriber.events[index] != want[index] {
			t.Fatalf("event %d = %+v, want %+v", index, subscriber.events[index], want[index])
		}
	}
	if subscriber.ctx != ctx {
		t.Fatal("subscriber did not receive the publishing context")
	}
}
