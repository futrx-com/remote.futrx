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
	first, err := store.PrepareRun(ctx, chat.RunPreparation{ChatID: meta.ID, RunID: "attempt-1", TurnID: "turn-1", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RecordRunInput(ctx, first.RunID, "input", "prompt_context_v1"); err != nil {
		t.Fatal(err)
	}
	if err = store.AcceptRunInput(ctx, first.RunID, chat.InputAcceptance{NativeSessionID: "native-session", Evidence: "native-ack"}); err != nil {
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
	next, err := store.PrepareRun(ctx, chat.RunPreparation{ChatID: meta.ID, RunID: "attempt-2", TurnID: "turn-2", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if next.ResumeID != "" || next.SessionID == first.SessionID {
		t.Fatal("resumed uncertain native delivery")
	}
	if len(next.History.Records) != 1 || next.History.Records[0].Text != "retained request" {
		t.Fatal(next.History)
	}
	var status string
	if err = store.db.QueryRow(`SELECT status FROM runs WHERE id='attempt-1'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "uncertain" {
		t.Fatal(status)
	}
}

func TestAcceptanceDoesNotAdvancePastDeliveredRange(t *testing.T) {
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
	prepared, err := store.PrepareRun(ctx, chat.RunPreparation{ChatID: meta.ID, RunID: "run-1", TurnID: "turn-1", Provider: chat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendEvent(ctx, meta.ID, chat.Event{Type: "user", Text: "new"}); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordRunInput(ctx, prepared.RunID, "old + new", "prompt_context_v1"); err != nil {
		t.Fatal(err)
	}
	if err = store.AcceptRunInput(ctx, prepared.RunID, chat.InputAcceptance{NativeSessionID: "native", Evidence: "native-ack"}); err != nil {
		t.Fatal(err)
	}
	var bookmark int64
	if err = store.db.QueryRow(`SELECT history_through_seq FROM harness_sessions WHERE id=?`, prepared.SessionID).Scan(&bookmark); err != nil {
		t.Fatal(err)
	}
	if bookmark != 1 {
		t.Fatalf("acceptance bookmark=%d, want historical boundary 1", bookmark)
	}
	var prompt string
	if err = store.db.QueryRow(`SELECT b.content FROM content_blobs b JOIN runs r ON r.input_blob_id=b.id WHERE r.id=?`, prepared.RunID).Scan(&prompt); err != nil {
		t.Fatal(err)
	}
	if prompt != "old + new" {
		t.Fatal(prompt)
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
	prepared, err := store.PrepareRun(ctx, chat.RunPreparation{ChatID: meta.ID, RunID: "run-large", TurnID: "turn-large", Provider: chat.ProviderClaude})
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
	_, err = store.PrepareRun(ctx, chat.RunPreparation{ChatID: meta.ID, RunID: "failed-read", TurnID: "turn", Provider: chat.ProviderClaude})
	if err == nil {
		t.Fatal("corrupt history was accepted")
	}
	var status, state string
	if err = store.db.QueryRow(`SELECT r.status,s.state FROM runs r JOIN harness_sessions s ON s.id=r.harness_session_id WHERE r.id='failed-read'`).Scan(&status, &state); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || state != "uncertain" {
		t.Fatalf("attempt left resumable: status=%s state=%s", status, state)
	}
}
