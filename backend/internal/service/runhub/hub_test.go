package runhub

import (
	"context"
	"testing"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
)

func TestHubSubscribeReplaysAndBroadcasts(t *testing.T) {
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), servicechat.Meta{ID: "abcd"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(context.Background(), "abcd", servicechat.Event{T: 1, Type: "user", Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	hub := New(store)
	sub, err := hub.Subscribe(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	if ev := receiveEvent(t, sub); ev.Type != "user" || ev.Text != "hi" {
		t.Fatalf("unexpected replay event: %#v", ev)
	}
	if ev := receiveEvent(t, sub); ev.Type != "sync" || ev.Running {
		t.Fatalf("unexpected sync event: %#v", ev)
	}

	hub.Emit("abcd", servicechat.Event{T: 2, Type: "assistant_text", Text: "hello"})
	if ev := receiveEvent(t, sub); ev.Type != "assistant_text" || ev.Text != "hello" {
		t.Fatalf("unexpected broadcast event: %#v", ev)
	}
}

func TestHubSubscribeAfterOnlyReplaysMissingEvents(t *testing.T) {
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), servicechat.Meta{ID: "abcd"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(context.Background(), "abcd", servicechat.Event{T: 1, Type: "user", Text: "first"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(context.Background(), "abcd", servicechat.Event{T: 2, Type: "assistant_text", Text: "second"}); err != nil {
		t.Fatal(err)
	}

	hub := New(store)
	sub, err := hub.SubscribeAfter(context.Background(), "abcd", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	if ev := receiveEvent(t, sub); ev.Type != "assistant_text" || ev.Seq != 2 {
		t.Fatalf("unexpected replay event: %#v", ev)
	}
	if ev := receiveEvent(t, sub); ev.Type != "sync" || ev.Running {
		t.Fatalf("unexpected sync event: %#v", ev)
	}
}

func TestHubAllowsOnlyOneRunPerChat(t *testing.T) {
	hub := New(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runID, ok := hub.StartRun("abcd", cancel)
	if !ok {
		t.Fatal("first run should start")
	}
	if !hub.IsRunning("abcd") {
		t.Fatal("expected run to be marked active")
	}
	if _, ok := hub.StartRun("abcd", func() {}); ok {
		t.Fatal("second run should be rejected")
	}
	if !hub.CancelRun("abcd") {
		t.Fatal("expected active run to cancel")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("cancel was not called")
	}

	if !hub.IsRunning("abcd") {
		t.Fatal("cancelled run should retain the lock until it finishes")
	}
	if _, ok := hub.StartRun("abcd", func() {}); ok {
		t.Fatal("new run should not start while cancellation is still draining")
	}
	hub.FinishRun("abcd", runID)
	if hub.IsRunning("abcd") {
		t.Fatal("finished cancelled run should release the lock")
	}
	if _, ok := hub.StartRun("abcd", func() {}); !ok {
		t.Fatal("new run should start after cancelled run finishes")
	}
}

func TestHubRepeatedCancelIsIdempotentUntilRunFinishes(t *testing.T) {
	hub := New(nil)
	cancelled := 0
	runID, ok := hub.StartRun("abcd", func() { cancelled++ })
	if !ok {
		t.Fatal("run should start")
	}

	if !hub.CancelRun("abcd") || !hub.CancelRun("abcd") {
		t.Fatal("repeated cancellation should find the draining run")
	}
	if cancelled != 1 {
		t.Fatalf("cancel calls = %d, want 1", cancelled)
	}

	hub.FinishRun("abcd", runID)
	if hub.CancelRun("abcd") {
		t.Fatal("finished run should no longer be cancellable")
	}
}

func TestHubPublishesRunningTransitions(t *testing.T) {
	hub := New(nil)
	updates := make(chan bool, 2)
	hub.SetRunningSubscriber(func(id servicechat.ID, running bool) {
		if id == "abcd" {
			updates <- running
		}
	})

	runID, ok := hub.StartRun("abcd", func() {})
	if !ok {
		t.Fatal("run should start")
	}
	if running := receiveRunning(t, updates); !running {
		t.Fatal("expected running=true update")
	}

	if !hub.CancelRun("abcd") {
		t.Fatal("run should accept cancellation")
	}
	select {
	case running := <-updates:
		t.Fatalf("cancel published premature running=%v update", running)
	default:
	}

	hub.FinishRun("abcd", runID)
	if running := receiveRunning(t, updates); running {
		t.Fatal("expected running=false update")
	}
}

func TestHubEmitAllowsAppendNotificationsToReadRunning(t *testing.T) {
	running := make(chan bool, 1)
	var hub *Hub
	store := callbackStore{
		append: func() {
			running <- hub.IsRunning("abcd")
		},
	}
	hub = New(store)

	runID, ok := hub.StartRun("abcd", func() {})
	if !ok {
		t.Fatal("run should start")
	}

	done := make(chan struct{})
	go func() {
		hub.Emit("abcd", servicechat.Event{T: 1, Type: "user", Text: "hi"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("emit deadlocked while append notification read running state")
	}
	if isRunning := receiveRunning(t, running); !isRunning {
		t.Fatal("expected append notification to observe running chat")
	}

	hub.FinishRun("abcd", runID)
}

type callbackStore struct {
	append func()
}

func (s callbackStore) ReadEvents(ctx context.Context, chatID servicechat.ID) ([]servicechat.Event, error) {
	return nil, nil
}

func (s callbackStore) ReadEventsAfter(
	ctx context.Context,
	chatID servicechat.ID,
	afterSeq int64,
) ([]servicechat.Event, error) {
	return nil, nil
}

func (s callbackStore) AppendEvent(
	ctx context.Context,
	chatID servicechat.ID,
	ev servicechat.Event,
) (servicechat.Event, error) {
	if s.append != nil {
		s.append()
	}
	ev.Seq = 1
	return ev, nil
}

func receiveEvent(t *testing.T, sub *Subscription) servicechat.Event {
	t.Helper()
	select {
	case ev := <-sub.Events():
		return ev
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return servicechat.Event{}
	}
}

func receiveRunning(t *testing.T, updates <-chan bool) bool {
	t.Helper()
	select {
	case running := <-updates:
		return running
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for running update")
		return false
	}
}
