package sqlitechat

import (
	"context"
	"encoding/json"
	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"strings"
	"testing"
)

func TestPendingDeliveryAfterReopenStartsFresh(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(ctx, chat.Meta{ID: "abcdef123456", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendEvent(ctx, meta.ID, chat.Event{Type: "user", Text: "retained request"}); err != nil {
		t.Fatal(err)
	}
	first, err := store.PrepareRun(ctx, chat.RunPreparation{ChatID: meta.ID, RunID: "attempt-1", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RecordRunInput(ctx, first.RunReference, "prompt_context_v1"); err != nil {
		t.Fatal(err)
	}
	if err = store.AcceptRunInput(ctx, first.RunReference, chat.InputAcceptance{NativeSessionID: "native-session", Evidence: "native-ack"}); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	next, err := store.PrepareRun(ctx, chat.RunPreparation{ChatID: meta.ID, RunID: "attempt-2", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if next.ResumeID != "" {
		t.Fatal("resumed uncertain native delivery")
	}
	if len(next.History.Records) != 1 || next.History.Records[0].Text != "retained request" {
		t.Fatal(next.History)
	}
	session := currentTestSession(t, store, meta.ID, chat.ProviderClaude, "")
	if session.State != "pending" || session.RunID != next.RunID || session.NativeSessionID != "" {
		t.Fatalf("old delivery survived new preparation: %+v", session)
	}
}

func TestAcceptanceLeavesSafeBookmarkUntilCompletion(t *testing.T) {
	ctx := context.Background()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	meta, err := store.Create(ctx, chat.Meta{ID: "abcdef123456", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendEvent(ctx, meta.ID, chat.Event{Type: "user", Text: "old"}); err != nil {
		t.Fatal(err)
	}
	prepared, err := store.PrepareRun(ctx, chat.RunPreparation{ChatID: meta.ID, RunID: "run-1", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendEvent(ctx, meta.ID, chat.Event{Type: "user", Text: "new"}); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordRunInput(ctx, prepared.RunReference, "prompt_context_v1"); err != nil {
		t.Fatal(err)
	}
	if err = store.AcceptRunInput(ctx, prepared.RunReference, chat.InputAcceptance{NativeSessionID: "native", Evidence: "native-ack"}); err != nil {
		t.Fatal(err)
	}
	session := currentTestSession(t, store, meta.ID, chat.ProviderClaude, "")
	if session.HistoryThroughSeq != 0 || session.State != "accepted" {
		t.Fatalf("unfinished input marked safe: %+v", session)
	}
	if err = store.FinishRun(ctx, prepared.RunReference, chat.RunOutcome{Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	session = currentTestSession(t, store, meta.ID, chat.ProviderClaude, "")
	if session.HistoryThroughSeq != 2 || session.State != "ready" || session.RunID != "" || session.DeliveryMethod != "" {
		t.Fatalf("completed session: %+v", session)
	}
}

func TestHandoffRangeKeepsLargeContentAndExcludesTelemetry(t *testing.T) {
	ctx := context.Background()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	meta, err := store.Create(ctx, chat.Meta{ID: "abcdef123456", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("history λ ", 10000)
	for _, event := range []chat.Event{
		{Type: "user", Text: "original request"},
		{Type: "provider_event", Data: json.RawMessage(`{"diagnostic":"` + strings.Repeat("opaque", 20000) + `"}`)},
		{Type: "assistant_text", Text: text, Provider: chat.ProviderCodex},
	} {
		if _, err = store.AppendEvent(ctx, meta.ID, event); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := store.PrepareRun(ctx, chat.RunPreparation{ChatID: meta.ID, RunID: "run-large", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.History.Records) != 2 {
		t.Fatal(prepared.History)
	}
	if prepared.History.Records[1].Text != text {
		t.Fatal("history truncated")
	}
	if prepared.History.ThroughSeq != 3 {
		t.Fatal("filtered records changed history bookmark")
	}
}

func TestPrepareRunHistoryReadFailureFinalizesAttempt(t *testing.T) {
	ctx := context.Background()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	meta, err := store.Create(ctx, chat.Meta{ID: "abcdef123456", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendEvent(ctx, meta.ID, chat.Event{Type: "user", Text: "history"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec(`UPDATE events SET payload_json='invalid-json'`); err != nil {
		t.Fatal(err)
	}
	_, err = store.PrepareRun(ctx, chat.RunPreparation{ChatID: meta.ID, RunID: "failed-read", Provider: chat.ProviderClaude})
	if err == nil {
		t.Fatal("corrupt history was accepted")
	}
	session := currentTestSession(t, store, meta.ID, chat.ProviderClaude, "")
	if session.State != "uncertain" || session.RunID != "" {
		t.Fatalf("attempt left resumable: %+v", session)
	}
}

func currentTestSession(t *testing.T, store *Store, id chat.ID, provider chat.Provider, account string) sessionState {
	t.Helper()
	_, _, sessions, err := readSessions(context.Background(), store.db, id)
	if err != nil {
		t.Fatal(err)
	}
	session, ok := sessions[sessionKey(provider, account)]
	if !ok {
		t.Fatalf("missing current session for %s/%s", provider, account)
	}
	return session
}

func TestLateDeliveryCannotChangeReplacementRunOrBranch(t *testing.T) {
	for _, change := range []string{"run", "branch"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			store := testStore(t)
			makeChat(t, store, "abcd")
			appendEvents(t, store, "abcd", chat.Event{T: 10, Type: "user", Text: "history"})
			old, err := store.PrepareRun(ctx, chat.RunPreparation{ChatID: "abcd", RunID: "old", Provider: chat.ProviderClaude})
			if err != nil {
				t.Fatal(err)
			}
			if err = store.RecordRunInput(ctx, old.RunReference, "prompt_context"); err != nil {
				t.Fatal(err)
			}
			if change == "branch" {
				if err = store.RewindBranch(ctx, "abcd", 10); err != nil {
					t.Fatal(err)
				}
			}
			next, err := store.PrepareRun(ctx, chat.RunPreparation{ChatID: "abcd", RunID: "new", Provider: chat.ProviderClaude})
			if err != nil {
				t.Fatal(err)
			}
			if err = store.RecordRunInput(ctx, next.RunReference, "prompt_context"); err != nil {
				t.Fatal(err)
			}
			if err = store.AcceptRunInput(ctx, next.RunReference, chat.InputAcceptance{NativeSessionID: "new-native", Evidence: "ack"}); err != nil {
				t.Fatal(err)
			}
			if err = store.AcceptRunInput(ctx, old.RunReference, chat.InputAcceptance{NativeSessionID: "old-native", Evidence: "late-ack"}); err != nil {
				t.Fatal(err)
			}
			if err = store.FinishRun(ctx, old.RunReference, chat.RunOutcome{Status: "completed", NativeSessionID: "old-native"}); err != nil {
				t.Fatal(err)
			}
			session := currentTestSession(t, store, "abcd", chat.ProviderClaude, "")
			if session.NativeSessionID != "new-native" || session.RunID != "new" || session.State != "accepted" {
				t.Fatalf("late callback changed newer session: %+v", session)
			}
			if err = store.RecordRunInput(ctx, old.RunReference, "prompt_context"); err == nil {
				t.Fatal("stale run may still deliver input")
			}
			if err = store.FinishRun(ctx, next.RunReference, chat.RunOutcome{Status: "completed"}); err != nil {
				t.Fatal(err)
			}
			if err = store.FinishRun(ctx, old.RunReference, chat.RunOutcome{Status: "failed"}); err != nil {
				t.Fatal(err)
			}
			session = currentTestSession(t, store, "abcd", chat.ProviderClaude, "")
			if session.NativeSessionID != "new-native" || session.State != "ready" {
				t.Fatalf("late failure changed completed session: %+v", session)
			}
		})
	}
}

func TestSessionRequiresDeliveryAndAcceptanceBeforeCompletion(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	makeChat(t, store, "abcd")
	run, err := store.PrepareRun(ctx, chat.RunPreparation{ChatID: "abcd", RunID: "run", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AcceptRunInput(ctx, run.RunReference, chat.InputAcceptance{NativeSessionID: "native", Evidence: "ack"}); err == nil {
		t.Fatal("input accepted before delivery was recorded")
	}
	if err = store.RecordRunInput(ctx, run.RunReference, "prompt_context"); err != nil {
		t.Fatal(err)
	}
	if err = store.AcceptRunInput(ctx, run.RunReference, chat.InputAcceptance{NativeSessionID: "native"}); err == nil {
		t.Fatal("input accepted without provider evidence")
	}
	if err = store.FinishRun(ctx, run.RunReference, chat.RunOutcome{Status: "completed", NativeSessionID: "native"}); err != nil {
		t.Fatal(err)
	}
	if session := currentTestSession(t, store, "abcd", chat.ProviderClaude, ""); session.State != "uncertain" {
		t.Fatalf("unaccepted completion is resumable: %+v", session)
	}
}
