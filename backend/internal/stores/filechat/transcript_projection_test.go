package filechat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func TestTranscriptProjectionCollapsesTelemetryAndLoadsFullResponses(t *testing.T) {
	root := t.TempDir()
	toolOutput := strings.Repeat("tool-result-🙂\n", 4_000)
	subagentOutput := strings.Repeat("subagent-tool-output\n", 3_000)
	subagentMessage := strings.Repeat("final subagent report\n", 3_000)
	firstCollaboration := json.RawMessage(`{
		"type":"subagentThread",
		"tools":[{"id":"child-tool","name":"exec","output":"working"}],
		"agentsStates":{"child":{"status":"inProgress"}}
	}`)
	finalCollaboration, err := json.Marshal(map[string]any{
		"type": "subagentThread",
		"tools": []any{map[string]any{
			"id": "child-tool", "name": "exec", "status": "completed",
			"output": subagentOutput,
		}},
		"agentsStates": map[string]any{
			"child": map[string]any{"status": "completed", "message": subagentMessage},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := []servicechat.Event{
		{Seq: 1, T: 1, Type: "user", TurnID: "turn-1", Text: "question"},
		{Seq: 2, T: 2, Type: "provider_event", TurnID: "turn-1", Data: json.RawMessage(`{"raw":"telemetry"}`)},
		{Seq: 3, T: 3, Type: "tool_use_start", TurnID: "turn-1", ID: "tool-1", Name: "Bash", Input: json.RawMessage(`{"command":"test"}`)},
		{Seq: 4, T: 4, Type: "collaboration", TurnID: "turn-1", ID: "child", Name: "spawn", Status: "inProgress", Data: firstCollaboration},
		{Seq: 5, T: 5, Type: "tool_use_end", TurnID: "turn-1", ID: "tool-1", Output: toolOutput},
		{Seq: 6, T: 6, Type: "collaboration", TurnID: "turn-1", ID: "child", Name: "spawn", Status: "completed", Data: finalCollaboration},
		{Seq: 7, T: 7, Type: "complete", TurnID: "turn-1"},
		{Seq: 8, T: 8, Type: "user", TurnID: "turn-2", Text: "next question"},
		{Seq: 9, T: 9, Type: "complete", TurnID: "turn-2"},
	}
	writeStoredChat(t, root, "abcd", events)
	store := newIndexedTestStore(t, root)
	if _, err := store.index.syncChat(context.Background(), "abcd", store.eventsPath("abcd")); err != nil {
		t.Fatal(err)
	}

	page, err := store.ReadTranscriptPage(
		context.Background(), "abcd", servicechat.TranscriptPageQuery{Limit: 10},
	)
	if err != nil {
		t.Fatal(err)
	}
	if page.Indexing != nil || len(page.Turns) != 2 {
		t.Fatalf("projected page = %#v", page)
	}
	var projected []servicechat.Event
	for _, turn := range page.Turns {
		projected = append(projected, turn.Events...)
	}
	var collaborations int
	var toolRef, childToolRef, childMessageRef string
	for _, event := range projected {
		switch event.Type {
		case "provider_event":
			t.Fatal("provider telemetry leaked into transcript projection")
		case "tool_use_end":
			toolRef = event.OutputRef
			if !event.OutputTruncated || event.OutputBytes != int64(len(toolOutput)) || len(event.Output) >= len(toolOutput) {
				t.Fatalf("tool preview = %#v", event)
			}
		case "collaboration":
			collaborations++
			if event.Status != "completed" || event.Seq != 4 {
				t.Fatalf("collaboration did not retain position and latest state: %#v", event)
			}
			var data map[string]any
			if err := json.Unmarshal(event.Data, &data); err != nil {
				t.Fatal(err)
			}
			tool := data["tools"].([]any)[0].(map[string]any)
			childToolRef, _ = tool["outputRef"].(string)
			state := data["agentsStates"].(map[string]any)["child"].(map[string]any)
			childMessageRef, _ = state["messageRef"].(string)
		}
	}
	var collaborationEndSeq, collaborationTurnOrdinal int64
	if err := store.index.db.QueryRow(`
		SELECT end_seq, turn_ordinal
		FROM chat_transcript_items
		WHERE chat_id = ? AND item_key = ?`, "abcd", "collaboration:child",
	).Scan(&collaborationEndSeq, &collaborationTurnOrdinal); err != nil {
		t.Fatal(err)
	}
	if collaborationEndSeq != 6 || collaborationTurnOrdinal != 1 {
		t.Fatalf("collaboration index end=%d turn=%d", collaborationEndSeq, collaborationTurnOrdinal)
	}
	var contentTurnOrdinal int64
	if err := store.index.db.QueryRow(`
		SELECT turn_ordinal
		FROM chat_transcript_content_refs
		WHERE chat_id = ? AND content_id = ?`, "abcd", childToolRef,
	).Scan(&contentTurnOrdinal); err != nil {
		t.Fatal(err)
	}
	if contentTurnOrdinal != collaborationTurnOrdinal {
		t.Fatalf("content reference turn=%d, want %d", contentTurnOrdinal, collaborationTurnOrdinal)
	}
	if collaborations != 1 || toolRef == "" || childToolRef == "" || childMessageRef == "" {
		t.Fatalf("projection refs: collaborations=%d tool=%q childTool=%q childMessage=%q", collaborations, toolRef, childToolRef, childMessageRef)
	}
	for ref, want := range map[string]string{
		toolRef:         toolOutput,
		childToolRef:    subagentOutput,
		childMessageRef: subagentMessage,
	} {
		if got := readAllTranscriptContent(t, store, "abcd", ref, 4_001); got != want {
			t.Fatalf("full content %q has %d bytes, want %d", ref, len(got), len(want))
		}
	}
}

func TestTranscriptProjectionPagesInsideOneLargeTurnByBytes(t *testing.T) {
	root := t.TempDir()
	events := []servicechat.Event{{Seq: 1, T: 1, Type: "user", TurnID: "turn-1", Text: "question"}}
	for seq := int64(2); seq <= 11; seq++ {
		events = append(events, servicechat.Event{
			Seq: seq, T: seq, Type: "assistant_text", TurnID: "turn-1",
			MessageID: "message-" + string(rune('a'+seq)), Text: strings.Repeat("x", 900),
		})
	}
	events = append(events, servicechat.Event{Seq: 12, T: 12, Type: "complete", TurnID: "turn-1"})
	writeStoredChat(t, root, "abcd", events)
	store := newIndexedTestStore(t, root)
	if _, err := store.index.syncChat(context.Background(), "abcd", store.eventsPath("abcd")); err != nil {
		t.Fatal(err)
	}

	before := int64(0)
	seen := make(map[int64]struct{})
	pages := 0
	for {
		page, err := store.ReadTranscriptPage(context.Background(), "abcd", servicechat.TranscriptPageQuery{
			Limit: 10, BeforeSeq: before, ByteLimit: 1_500,
		})
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, turn := range page.Turns {
			for _, event := range turn.Events {
				if _, duplicate := seen[event.Seq]; duplicate {
					t.Fatalf("duplicate sequence %d", event.Seq)
				}
				seen[event.Seq] = struct{}{}
			}
		}
		if !page.HasMore {
			break
		}
		if page.NextBefore <= 0 || page.NextBefore >= before && before > 0 {
			t.Fatalf("unsafe cursor: before=%d page=%#v", before, page)
		}
		before = page.NextBefore
	}
	if pages < 2 || len(seen) != len(events) {
		t.Fatalf("pages=%d sequences=%d want=%d", pages, len(seen), len(events))
	}
}

func TestTranscriptProjectionStartsBackgroundBackfillWithTailSequence(t *testing.T) {
	root := t.TempDir()
	writeStoredChat(t, root, "abcd", []servicechat.Event{
		{Seq: 41, T: 1, Type: "user", TurnID: "turn", Text: "question"},
		{Seq: 42, T: 2, Type: "complete", TurnID: "turn"},
	})
	store := newIndexedTestStore(t, root)
	page, err := store.ReadTranscriptPage(context.Background(), "abcd", servicechat.TranscriptPageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Indexing == nil || !page.Indexing.TailSeqKnown || page.LastSeq != 42 {
		t.Fatalf("initial background page = %#v", page)
	}
	deadline := time.Now().Add(2 * time.Second)
	for page.Indexing != nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		page, err = store.ReadTranscriptPage(context.Background(), "abcd", servicechat.TranscriptPageQuery{})
		if err != nil {
			t.Fatal(err)
		}
	}
	if page.Indexing != nil || len(page.Turns) != 1 {
		t.Fatalf("completed background page = %#v", page)
	}
}

func TestChatIndexCheckpointEndsOnARecordBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	size := configconstants.ChatIndexCheckpointBytes + 20
	if err := file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("abc\ndef"), configconstants.ChatIndexCheckpointBytes); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := chatIndexCheckpoint(path, 0, size)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint != configconstants.ChatIndexCheckpointBytes+4 {
		t.Fatalf("checkpoint = %d, want %d", checkpoint, configconstants.ChatIndexCheckpointBytes+4)
	}
}

func readAllTranscriptContent(
	t *testing.T,
	store *Store,
	chatID servicechat.ID,
	contentID string,
	limit int,
) string {
	t.Helper()
	var content strings.Builder
	var after int64
	for {
		page, err := store.ReadTranscriptContent(
			context.Background(), chatID, contentID, after, limit,
		)
		if err != nil {
			t.Fatal(err)
		}
		content.WriteString(page.Content)
		if page.Complete {
			return content.String()
		}
		if page.NextAfter <= after {
			t.Fatalf("content cursor did not advance: %#v", page)
		}
		after = page.NextAfter
	}
}
