package sqlitechat

import (
	"context"
	"errors"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func TestStoreListTracksMetadataChanges(t *testing.T) {
	store := testStore(t)
	if _, err := store.Create(context.Background(), servicechat.Meta{
		ID: "abcd", Title: "Existing", CreatedAt: 1, LastMessageAt: 10,
	}); err != nil {
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
		t.Fatalf("chat should start read, got lastReadAt=%d", list[0].LastReadAt)
	}
	if list[0].AccountID != "" || list[0].SetupRepair {
		t.Fatalf("chat gained recovery configuration: %+v", list[0])
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

func TestStorePreservesRecoveryConfigurationAfterReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, meta := range []servicechat.Meta{
		{ID: "abcd", Provider: servicechat.ProviderCodex, AccountID: "selected-account"},
		{ID: "beef", Provider: servicechat.ProviderCodex, AccountID: "selected-account", SetupRepair: true},
	} {
		if _, err := store.Create(ctx, meta); err != nil {
			t.Fatal(err)
		}
		if _, err := store.AppendEvent(ctx, meta.ID, servicechat.Event{Type: "user", Text: "repair setup"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	for _, id := range []servicechat.ID{"abcd", "beef"} {
		meta, err := reopened.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if meta.AccountID != "selected-account" || meta.SetupRepair != (id == "beef") {
			t.Fatalf("reopened chat lost recovery configuration: %+v", meta)
		}
	}
}

func TestStoreScanEventsHonorsCancellation(t *testing.T) {
	store := testStore(t)
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
	err := store.ScanEvents(ctx, "abcd", func(servicechat.Event) {
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
	store := testStore(t)
	if _, err := store.Create(context.Background(), servicechat.Meta{
		ID:            "abcd",
		CreatedAt:     10,
		LastMessageAt: 10,
		Sessions: servicechat.SessionIDs{
			servicechat.ProviderAntigravity: "agy-session",
			"future-harness":                "future-session",
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
	t.Cleanup(func() { _ = store.Close() })
	created, err := store.Create(context.Background(), servicechat.Meta{
		ID:       "abcd",
		Provider: servicechat.ProviderAntigravity,
		Sessions: servicechat.SessionIDs{
			servicechat.ProviderAntigravity: "agy-session",
			"future-harness":                "future-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.AntigravitySessionID != "agy-session" {
		t.Fatalf("legacy Antigravity session = %q", created.AntigravitySessionID)
	}
	event := servicechat.Event{T: 10, Type: "session"}
	event.SetSession("future-harness", "event-session")
	if _, err := store.AppendEvent(context.Background(), "abcd", event); err != nil {
		t.Fatal(err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	meta, err := reopened.Get(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if meta.SessionID(servicechat.ProviderAntigravity) != "agy-session" || meta.SessionID("future-harness") != "future-session" {
		t.Fatalf("reloaded sessions = %#v", meta.Sessions)
	}
	events, err := reopened.ReadEvents(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Provider != "future-harness" || events[0].SessionID != "event-session" {
		t.Fatalf("reloaded events = %#v", events)
	}
}

func TestStorePersistsSelectedSkills(t *testing.T) {
	store := testStore(t)

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

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(store.root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	loaded, err = reopened.Get(context.Background(), "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.SelectedSkills) != 2 || loaded.SelectedSkills[0].Provider != servicechat.ProviderCodex {
		t.Fatalf("reloaded skills = %#v", loaded.SelectedSkills)
	}
}

func TestStorePersistsHarnessSelectionsAfterReopen(t *testing.T) {
	store := testStore(t)
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

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(store.root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
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

func TestPromptAuthorsSurviveReopenAndFork(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	meta, err := store.Create(ctx, servicechat.Meta{ID: "abcd", Provider: servicechat.ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	authors := []string{"alice@example.com", "bob@example.com"}
	for i, author := range authors {
		if _, err := store.AppendEvent(ctx, meta.ID, servicechat.Event{T: int64(i + 1), Type: "user", Text: "hello", UserEmail: author}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	forked, err := reopened.ForkChat(ctx, meta.ID, servicechat.Meta{ID: "beef", Provider: servicechat.ProviderClaude}, -1)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []servicechat.ID{meta.ID, forked.ID} {
		events, err := reopened.ReadEvents(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != len(authors) {
			t.Fatalf("chat %s lost messages", id)
		}
		page, err := reopened.ReadTranscriptPage(ctx, id, servicechat.TranscriptPageQuery{})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Turns) != len(authors) {
			t.Fatalf("chat %s lost transcript turns: %#v", id, page)
		}
		for i, turn := range page.Turns {
			if len(turn.Events) != 1 || turn.Events[0].UserEmail != authors[i] {
				t.Fatalf("chat %s lost transcript author: %#v", id, turn)
			}
		}
		for i, event := range events {
			if event.UserEmail != authors[i] {
				t.Fatalf("chat %s lost author: %#v", id, event)
			}
		}
	}
}
