package filechat

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func TestStoreListUsesCachedMetadata(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "chats", "abcd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "meta.json"),
		[]byte(`{"id":"abcd","title":"Existing","createdAt":1,"lastMessageAt":10}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	list, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "abcd" || list[0].Title != "Existing" {
		t.Fatalf("loaded list = %#v", list)
	}
	if list[0].LastReadAt != 10 {
		t.Fatalf("legacy chat should start read, got lastReadAt=%d", list[0].LastReadAt)
	}

	if _, err := store.Create(context.Background(), servicechat.Meta{ID: "beef", Title: "New", CreatedAt: 2, LastMessageAt: 20}); err != nil {
		t.Fatal(err)
	}
	list, err = store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "beef" {
		t.Fatalf("created list = %#v", list)
	}

	if _, err := store.Update(context.Background(), "abcd", func(m *servicechat.Meta) {
		m.Title = "Renamed"
		m.LastMessageAt = 30
	}); err != nil {
		t.Fatal(err)
	}
	list, err = store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if list[0].ID != "abcd" || list[0].Title != "Renamed" {
		t.Fatalf("updated list = %#v", list)
	}

	if _, err := store.AppendEvent(context.Background(), "beef", servicechat.Event{T: 40, Type: "user", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	list, err = store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if list[0].ID != "beef" || list[0].LastMessageAt != 40 {
		t.Fatalf("append list = %#v", list)
	}

	if err := store.Delete(context.Background(), "beef"); err != nil {
		t.Fatal(err)
	}
	list, err = store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "abcd" {
		t.Fatalf("deleted list = %#v", list)
	}
}

func TestStoreReadsEventPages(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), servicechat.Meta{ID: "abcd"}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		if _, err := store.AppendEvent(context.Background(), "abcd", servicechat.Event{
			T:    int64(i),
			Type: "user",
			Text: string(rune('a' + i - 1)),
		}); err != nil {
			t.Fatal(err)
		}
	}

	page, err := store.ReadEventsPage(context.Background(), "abcd", servicechat.EventPageQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 2 || page.Events[0].Seq != 4 || page.Events[1].Seq != 5 {
		t.Fatalf("latest page = %#v", page)
	}
	if !page.HasMore || page.NextBefore != 4 || page.LastSeq != 5 {
		t.Fatalf("latest page cursors = %#v", page)
	}

	older, err := store.ReadEventsPage(context.Background(), "abcd", servicechat.EventPageQuery{Limit: 2, BeforeSeq: page.NextBefore})
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Events) != 2 || older.Events[0].Seq != 2 || older.Events[1].Seq != 3 {
		t.Fatalf("older page = %#v", older)
	}

	after, err := store.ReadEventsAfter(context.Background(), "abcd", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 2 || after[0].Seq != 4 || after[1].Seq != 5 {
		t.Fatalf("after = %#v", after)
	}
}

func TestStoreScansRawEventsInStorageOrder(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), servicechat.Meta{ID: "abcd"}); err != nil {
		t.Fatal(err)
	}
	for _, event := range []servicechat.Event{
		{T: 1, Type: "user", TurnID: "turn-1", Text: "question"},
		{T: 2, Type: "assistant_text", TurnID: "turn-1", MessageID: "message-1", Text: "answer"},
	} {
		if _, err := store.AppendEvent(context.Background(), "abcd", event); err != nil {
			t.Fatal(err)
		}
	}

	var events []servicechat.Event
	if err := store.ScanEvents(context.Background(), "abcd", func(event servicechat.Event) {
		events = append(events, event)
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Seq != 1 || events[1].Seq != 2 ||
		events[0].TurnID != "turn-1" || events[1].TurnID != "turn-1" ||
		events[1].MessageID != "message-1" {
		t.Fatalf("scanned events = %#v", events)
	}
}

func TestStoreScanEventsHonorsCancellation(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), servicechat.Meta{ID: "abcd"}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.AppendEvent(context.Background(), "abcd", servicechat.Event{
		T: 1, Type: "user", Text: "hello",
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	visited := false
	err = store.ScanEvents(ctx, "abcd", func(servicechat.Event) {
		visited = true
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ScanEvents error = %v, want context.Canceled", err)
	}
	if visited {
		t.Fatal("ScanEvents visited an event after cancellation")
	}
}

func TestStoreRewindClearsProviderSessionIDs(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), servicechat.Meta{
		ID:            "abcd",
		CreatedAt:     10,
		LastMessageAt: 10,
		Sessions: servicechat.SessionIDs{
			servicechat.ProviderAntigravity: "agy-session",
			"future-agent":                  "future-session",
		},
		ClaudeSessionID: "claude-session",
		CodexSessionID:  "codex-session",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.AppendEvent(context.Background(), "abcd", servicechat.Event{
		T:    20,
		Type: "user",
		Text: "keep",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(context.Background(), "abcd", servicechat.Event{
		T:    30,
		Type: "user",
		Text: "rewind from here",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(context.Background(), "abcd", servicechat.Event{
		T:    40,
		Type: "assistant_text",
		Text: "remove",
	}); err != nil {
		t.Fatal(err)
	}

	kept, err := store.TruncateEventsBefore(context.Background(), "abcd", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 || kept[0].Text != "keep" {
		t.Fatalf("kept events = %#v", kept)
	}

	events, err := store.ReadEvents(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Text != "keep" {
		t.Fatalf("persisted events = %#v", events)
	}

	meta, err := store.Get(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Sessions) != 0 || meta.ClaudeSessionID != "" || meta.CodexSessionID != "" ||
		meta.AntigravitySessionID != "" {
		t.Fatalf("session ids were not cleared: %#v", meta)
	}
	if meta.LastMessageAt != 20 {
		t.Fatalf("LastMessageAt = %d, want 20", meta.LastMessageAt)
	}
}

func TestStorePersistsGenericAndLegacySessionShapes(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(context.Background(), servicechat.Meta{
		ID:       "abcd",
		Provider: servicechat.ProviderAntigravity,
		Sessions: servicechat.SessionIDs{
			servicechat.ProviderAntigravity: "agy-session",
			"future-agent":                  "future-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.AntigravitySessionID != "agy-session" {
		t.Fatalf("legacy Antigravity session = %q", created.AntigravitySessionID)
	}
	event := servicechat.Event{T: 10, Type: "session"}
	event.SetSession("future-agent", "event-session")
	if _, err := store.AppendEvent(context.Background(), "abcd", event); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := reopened.Get(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if meta.SessionID(servicechat.ProviderAntigravity) != "agy-session" || meta.SessionID("future-agent") != "future-session" {
		t.Fatalf("reloaded sessions = %#v", meta.Sessions)
	}
	events, err := reopened.ReadEvents(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Provider != "future-agent" || events[0].SessionID != "event-session" {
		t.Fatalf("reloaded events = %#v", events)
	}
}

func TestStoreImportsLegacySessionFields(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "chats", "abcd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "meta.json"),
		[]byte(`{"id":"abcd","title":"Legacy","createdAt":1,"lastMessageAt":1,"antigravitySessionId":"agy-legacy"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "events.jsonl"),
		[]byte("{\"t\":2,\"type\":\"session\",\"kimiSessionId\":\"kimi-legacy\"}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Get(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if meta.SessionID(servicechat.ProviderAntigravity) != "agy-legacy" {
		t.Fatalf("legacy metadata sessions = %#v", meta.Sessions)
	}
	events, err := store.ReadEvents(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Provider != servicechat.ProviderKimi || events[0].SessionID != "kimi-legacy" {
		t.Fatalf("legacy event = %#v", events)
	}
}

func TestStorePersistsSelectedSkills(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.Create(context.Background(), servicechat.Meta{
		ID:       "abcd",
		Provider: servicechat.ProviderCodex,
		SelectedSkills: []servicechat.SkillRef{
			{Name: "Custom Skill", Command: "custom", Source: "user"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.SelectedSkills) != 1 || created.SelectedSkills[0].Provider != servicechat.ProviderCodex {
		t.Fatalf("created skills = %#v", created.SelectedSkills)
	}

	loaded, err := store.Get(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.SelectedSkills) != 1 || loaded.SelectedSkills[0].Command != "custom" {
		t.Fatalf("loaded skills = %#v", loaded.SelectedSkills)
	}

	updated, err := store.Update(context.Background(), "abcd", func(m *servicechat.Meta) {
		m.SelectedSkills = append(m.SelectedSkills, servicechat.SkillRef{
			Name:     "Review",
			Command:  "review",
			Provider: servicechat.ProviderCodex,
			Source:   "project",
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.SelectedSkills) != 2 || updated.SelectedSkills[1].Source != "project" {
		t.Fatalf("updated skills = %#v", updated.SelectedSkills)
	}

	reopened, err := New(store.root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = reopened.Get(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.SelectedSkills) != 2 || loaded.SelectedSkills[0].Provider != servicechat.ProviderCodex {
		t.Fatalf("reloaded skills = %#v", loaded.SelectedSkills)
	}
}

func TestStorePersistsAgentSelectionsAcrossInstances(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), servicechat.Meta{
		ID:              "abcd",
		Provider:        servicechat.ProviderClaude,
		Model:           "claude-opus-current",
		Mode:            "plan",
		ReasoningEffort: "high",
		ServiceTier:     "fast",
		ProjectID:       "project-1",
	}); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(store.root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Get(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Provider != servicechat.ProviderClaude ||
		loaded.Model != "claude-opus-current" ||
		loaded.Mode != "plan" ||
		loaded.ReasoningEffort != "high" ||
		loaded.ServiceTier != "fast" ||
		loaded.ProjectID != "project-1" {
		t.Fatalf("reloaded selections = %#v", loaded)
	}
}
