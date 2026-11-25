package runhub

import (
	"context"
	"testing"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/sqlitechat"
)

func TestSubscribeAfterCompletionDoesNotReplayNativeCheckpoint(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.Create(ctx, chat.Meta{ID: "abcd", Provider: chat.ProviderClaude}); err != nil {
		t.Fatal(err)
	}
	run, err := store.PrepareRun(ctx, chat.RunPreparation{ChatID: "abcd", RunID: "run", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(ctx, "abcd", chat.Event{Type: "user", Text: "request"}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRunInput(ctx, run.RunReference, "prompt_context_v1"); err != nil {
		t.Fatal(err)
	}
	if err := store.AcceptRunInput(ctx, run.RunReference, chat.InputAcceptance{NativeSessionID: "native", Evidence: "native_ack"}); err != nil {
		t.Fatal(err)
	}
	completion, err := store.AppendEvent(ctx, "abcd", chat.Event{Type: "complete"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishRun(ctx, run.RunReference, chat.RunOutcome{Status: "completed", NativeCheckpoint: "claude:done"}); err != nil {
		t.Fatal(err)
	}
	// A connected client saw completion live, but internal checkpoint storage
	// happens afterward. Reconnecting at that cursor must leave the chat idle.
	hub := New(store)
	sub, err := hub.SubscribeAfter(ctx, "abcd", completion.Seq)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if event := receiveEvent(t, sub); event.Type != "sync" || event.Running {
		t.Fatalf("idle reconnect received internal bookkeeping: %+v", event)
	}
	select {
	case event := <-sub.Events():
		t.Fatalf("unexpected replay after idle sync: %+v", event)
	default:
	}
}
