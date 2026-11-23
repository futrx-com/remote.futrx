package chat

import (
	"context"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/model/conversation"
)

// RunPreparation is captured before appending the current user message. On a
// missing-session retry ThroughSeq keeps the same history boundary so that the
// new user message is never also replayed as historical context.
type RunPreparation struct {
	ChatID          ID
	RunID           string
	Provider        Provider
	AccountID       string
	ForceFresh      bool
	AllowNativeFork bool
	ThroughSeq      *int64
}

// RunReference identifies the current delivery without keeping an attempt ledger.
// Branch and run identity prevent stale callbacks from advancing newer sessions.
type RunReference struct {
	ChatID    ID
	BranchID  string
	RunID     string
	Provider  Provider
	AccountID string
}

type PreparedRun struct {
	RunReference
	ResumeID             string
	NativeCheckpoint     string
	NativeConversationID string
	History              conversation.History
}

type InputAcceptance struct {
	NativeSessionID string
	Evidence        string
}

type RunOutcome struct {
	Status           string // completed, failed, interrupted, or uncertain
	NativeSessionID  string
	NativeCheckpoint string
}

// ConversationRuntime marks native sessions pending before execution and records
// acceptance independently of terminal output. Implementations must not treat
// a session-created event alone as proof that a prompt was consumed.
type ConversationRuntime interface {
	PrepareRun(context.Context, RunPreparation) (PreparedRun, error)
	RecordRunInput(context.Context, RunReference, string) error
	AcceptRunInput(context.Context, RunReference, InputAcceptance) error
	FinishRun(context.Context, RunReference, RunOutcome) error
}

// ConversationRecords selects only portable conversation material. Native
// protocol envelopes remain stored separately, not replayed as executable calls.
// Adjacent text fragments with identical provenance are assembled byte-for-byte
// into one message. This preserves words split across deltas without repeating
// a JSON metadata wrapper for every token; it does not summarize text.
func ConversationRecords(events []Event) []conversation.Record {
	records := make([]conversation.Record, 0, len(events))
	var assistantText strings.Builder
	flushAssistant := func() {
		if len(records) > 0 && records[len(records)-1].Kind == "assistant" {
			records[len(records)-1].Text = assistantText.String()
			assistantText = strings.Builder{}
		}
	}
	for _, event := range events {
		record := conversation.Record{
			Seq: event.Seq, EndSeq: event.Seq, TurnID: event.TurnID, Provider: string(event.Provider),
			MessageID: event.MessageID, AccountID: event.AccountID,
		}
		switch event.Type {
		case "user":
			record.Kind, record.Text = "user", event.Text
		case "assistant_text":
			record.Kind, record.Text = "assistant", event.Text
		case "tool_use_start":
			record.Kind, record.ToolCallID, record.Name, record.Input = "tool_call", event.ID, event.Name, event.Input
		case "tool_use_end":
			record.Kind, record.ToolCallID, record.Output, record.ContentRef, record.IsError = "tool_result", event.ID, event.Output, event.OutputRef, event.IsError
		case "attachment":
			record.Kind, record.Name, record.ContentRef, record.Data = "attachment", event.Name, event.OutputRef, event.Data
		default:
			continue
		}
		if record.Kind == "assistant" && len(records) > 0 {
			previous := &records[len(records)-1]
			if previous.Kind == "assistant" && previous.TurnID == record.TurnID &&
				previous.Provider == record.Provider && previous.AccountID == record.AccountID &&
				previous.MessageID == record.MessageID {
				assistantText.WriteString(record.Text)
				previous.EndSeq = record.EndSeq
				continue
			}
		}
		flushAssistant()
		records = append(records, record)
		if record.Kind == "assistant" {
			assistantText.WriteString(record.Text)
		}
	}
	flushAssistant()
	return records
}
