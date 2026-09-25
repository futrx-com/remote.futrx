package prompt

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
)

func TestNotificationSummaryIsRemovedAcrossStreamingBoundaries(t *testing.T) {
	var filter notificationSummaryFilter
	var visible strings.Builder
	for _, chunk := range []string{
		"Implemented the route.\n<notifi",
		"cation_summary>Added stable ",
		"chat links and fixed refresh.</notifi",
		"cation_summary>\n",
	} {
		visible.WriteString(filter.text(chunk))
	}
	last, summary := filter.finish()
	visible.WriteString(last)
	if visible.String() != "Implemented the route.\n\n" {
		t.Fatalf("visible output = %q", visible.String())
	}
	if summary != "Added stable chat links and fixed refresh." {
		t.Fatalf("summary = %q", summary)
	}
}

func TestIncompleteNotificationSummaryIsNotPublished(t *testing.T) {
	var filter notificationSummaryFilter
	visible := filter.text("Answer. <notification_summary>private unfinished text")
	last, summary := filter.finish()
	if visible+last != "Answer. " || summary != "" {
		t.Fatalf("visible = %q, summary = %q", visible+last, summary)
	}
}

func TestNotificationSummaryIsByteBoundedAndValidUTF8(t *testing.T) {
	var filter notificationSummaryFilter
	filter.text(notificationSummaryOpen + strings.Repeat("🌍", 200) + notificationSummaryClose)
	_, summary := filter.finish()
	if len(summary) > maxNotificationSummaryBytes || !utf8.ValidString(summary) || summary == "" {
		t.Fatalf("summary has %d bytes, valid UTF-8 = %v", len(summary), utf8.ValidString(summary))
	}
}

func TestPromptPersistsOnlyVisibleAnswerAndCompletionSummary(t *testing.T) {
	ctx := context.Background()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	chat, err := store.Create(ctx, servicechat.Meta{ID: "abcdef12", Provider: servicechat.ProviderCodex})
	if err != nil {
		t.Fatal(err)
	}
	provider := &schedulePromptProvider{output: []string{
		"Fixed the route. <notifi",
		"cation_summary>Chat links survive refresh.</notification_",
		"summary>",
	}}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	service := New(store, nil, nil, runhub.New(store), registry)
	var summary string
	emit := func(event ChatEvent) {
		if event.Type == "complete" {
			summary = event.NotificationSummary
		}
		if _, err := store.AppendEvent(ctx, chat.ID, event); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.runPrompt(ctx, chat.ID, "fix routes", emit, emit); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(ctx, chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	var visible string
	for _, event := range events {
		if event.Type == "assistant_text" {
			visible += event.Text
		}
		if event.NotificationSummary != "" {
			t.Fatal("summary was persisted")
		}
	}
	if visible != "Fixed the route. " || summary != "Chat links survive refresh." {
		t.Fatalf("visible = %q, summary = %q", visible, summary)
	}
	if strings.Contains(visible, "notification_summary") {
		t.Fatal("private marker reached chat history")
	}
}
