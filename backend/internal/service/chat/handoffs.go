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
	ChatID     ID
	TurnID     string
	RunID      string
	Provider   Provider
	AccountID  string
	Model      string
	ForceFresh bool
	ThroughSeq *int64
}

type PreparedRun struct {
	RunID     string
	SessionID string // Remote's stable mapping ID, not the native session ID.
	ResumeID  string
	History   conversation.History
}

type InputAcceptance struct {
	NativeSessionID string
	NativeTurnID    string
	Evidence        string
}

type RunOutcome struct {
	Status          string // completed, failed, interrupted, or uncertain
	NativeSessionID string
	Error           string
}

// ConversationRuntime persists delivery attempts before execution and records
// acceptance independently of terminal output. Implementations must not treat
// a session-created event alone as proof that a prompt was consumed.
type ConversationRuntime interface {
	PrepareRun(context.Context, RunPreparation) (PreparedRun, error)
	RecordRunInput(context.Context, string, string, string) error
	AcceptRunInput(context.Context, string, InputAcceptance) error
	FinishRun(context.Context, string, RunOutcome) error
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
