package chat

import (
	"strings"
	"testing"
)

func TestConversationRecordsAssemblesTextWithoutChangingBytes(t *testing.T) {
	fragments := []string{"a partial wo", "rd\n", "λ", " and trailing spaces  "}
	var events []Event
	for index, fragment := range fragments {
		events = append(events, Event{Seq: int64(index + 1), TurnID: "turn", Type: "assistant_text", Provider: ProviderClaude, AccountID: "account", MessageID: "message", Text: fragment})
	}
	records := ConversationRecords(events)
	if len(records) != 1 {
		t.Fatalf("records=%#v", records)
	}
	record := records[0]
	if record.Text != strings.Join(fragments, "") || record.Seq != 1 || record.EndSeq != 4 {
		t.Fatalf("assembled record=%#v", record)
	}
	if record.Provider != "claude" || record.AccountID != "account" || record.TurnID != "turn" || record.MessageID != "message" {
		t.Fatal("provenance changed")
	}
}

func TestConversationRecordsDoesNotMergeAcrossIdentityOrToolBoundaries(t *testing.T) {
	base := Event{Type: "assistant_text", TurnID: "turn", Provider: ProviderClaude, AccountID: "account", MessageID: "message", Text: "text"}
	for _, change := range []struct {
		name  string
		apply func(*Event)
	}{
		{"turn", func(e *Event) { e.TurnID = "other" }},
		{"provider", func(e *Event) { e.Provider = ProviderCodex }},
		{"account", func(e *Event) { e.AccountID = "other" }},
		{"message", func(e *Event) { e.MessageID = "other" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			other := base
			change.apply(&other)
			records := ConversationRecords([]Event{base, other})
			if len(records) != 2 {
				t.Fatalf("different provenance merged: %#v", records)
			}
		})
	}
	for _, kind := range []string{"user", "tool_use_start", "tool_use_end", "attachment"} {
		t.Run(kind, func(t *testing.T) {
			records := ConversationRecords([]Event{base, {Type: kind, Text: "boundary", ID: "tool"}, base})
			if len(records) != 3 || records[0].Text != "text" || records[2].Text != "text" {
				t.Fatalf("crossed %s boundary: %#v", kind, records)
			}
		})
	}
}
