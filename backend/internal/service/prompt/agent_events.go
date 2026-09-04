package prompt

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
)

func (rnr *Service) emitAgentEvent(
	ctx context.Context,
	id servicechat.ID,
	provider agent.ProviderID,
	ev agent.Event,
	emit func(ChatEvent),
) {
	if ev.Provider == "" {
		ev.Provider = provider
	}
	if ev.Type == agent.EventSessionUpdated && ev.SessionID != "" {
		_, _ = rnr.store.Update(ctx, id, func(m *ChatMeta) {
			m.SetSessionID(servicechat.Provider(ev.Provider), ev.SessionID)
			m.ForkPending = false
			if m.Model == "" && ev.Model != "" {
				m.Model = ev.Model
			}
		})
	}

	chatEvent, ok := chatEventFromAgentEvent(ev)
	if ok {
		emit(chatEvent)
	}
}

func chatEventFromAgentEvent(ev agent.Event) (ChatEvent, bool) {
	t := ev.T
	if t == 0 {
		t = time.Now().UnixMilli()
	}

	out := ChatEvent{
		T:             t,
		Native:        ev.Native,
		InteractionID: ev.InteractionID,
		Status:        ev.Status,
	}
	switch ev.Type {
	case agent.EventSessionUpdated:
		out.Type = "session"
		out.SetSession(servicechat.Provider(ev.Provider), ev.SessionID)
	case agent.EventSystem:
		out.Type = "system"
		out.Subtype = ev.Subtype
		out.Data = ev.Data
	case agent.EventAssistantTextDelta:
		out.Type = "assistant_text"
		out.Text = ev.Text
		out.MessageID = agentEventMessageID(ev)
	case agent.EventReasoningDelta:
		out.Type = "thinking"
		out.Text = ev.Text
		out.MessageID = agentEventMessageID(ev)
	case agent.EventToolStarted:
		out.Type = "tool_use_start"
		out.ID = ev.ItemID
		out.Name = ev.ToolName
		out.Input = ev.Input
	case agent.EventToolCompleted:
		out.Type = "tool_use_end"
		out.ID = ev.ItemID
		out.Output = ev.Output
		out.IsError = ev.IsError
	case agent.EventInteractionRequest:
		out.Type = "interaction_request"
		out.ID = ev.InteractionID
		out.Name = ev.ToolName
		out.Input = ev.Input
	case agent.EventInteractionDone:
		out.Type = "interaction_resolved"
		out.ID = ev.InteractionID
		out.Name = ev.ToolName
	case agent.EventTurnStatus, agent.EventRunInterrupted:
		out.Type = "turn_status"
		out.Provider = servicechat.Provider(ev.Provider)
		out.Data = ev.Data
		if ev.Type == agent.EventRunInterrupted && out.Status == "" {
			out.Status = "interrupted"
		}
	case agent.EventCollaboration:
		out.Type = "collaboration"
		out.ID = ev.ItemID
		out.Name = ev.ToolName
		out.Data = ev.Data
	case agent.EventProviderNative:
		out.Type = "provider_event"
		if ev.Native != nil {
			out.Name = ev.Native.Method
		}
		out.Data = ev.Data
	case agent.EventUsageUpdated:
		out.Type = "usage_update"
		out.Usage = ev.Usage
	case agent.EventRunCompleted:
		out.Type = "complete"
		// Persist the provider per turn. A chat can switch agents, so its current
		// metadata is not sufficient for an offline usage-ledger rebuild.
		out.Provider = servicechat.Provider(ev.Provider)
		out.Usage = ev.Usage
	case agent.EventRunFailed, agent.EventError:
		out.Type = "error"
		out.Message = ev.Message
	default:
		return ChatEvent{}, false
	}
	return out, true
}

func agentEventMessageID(event agent.Event) string {
	if event.MessageID != "" {
		return event.MessageID
	}
	return event.ItemID
}

// ledgerRun is the run-scoped context a usage entry needs. It is captured
// once per prompt so the per-event hook stays allocation free.
type ledgerRun struct {
	runID     string
	chatID    servicechat.ID
	projectID string
	userEmail string
	provider  agent.ProviderID
	model     string
	scheduled bool
}

// recordRunUsage forwards a finished turn to the usage ledger. Only completed
// runs are recorded: a failed turn's token counts are not persisted in the
// chat event log, so counting them here would make the ledger impossible to
// rebuild from disk.
func (rnr *Service) recordRunUsage(ctx context.Context, run ledgerRun, ev agent.Event) {
	if rnr.usage == nil || ev.Type != agent.EventRunCompleted {
		return
	}
	at := ev.T
	if at == 0 {
		at = time.Now().UnixMilli()
	}
	provider := ev.Provider
	if provider == "" {
		provider = run.provider
	}
	// The turn is over, so a cancelled request context must not stop the
	// ledger write that describes it.
	rnr.usage.RecordRun(context.WithoutCancel(ctx), serviceusage.RunEvent{
		At:        at,
		ChatID:    string(run.chatID),
		ProjectID: run.projectID,
		RunID:     run.runID,
		UserEmail: run.userEmail,
		Provider:  string(provider),
		Model:     run.model,
		Usage:     ev.Usage,
		Scheduled: run.scheduled,
	})
}

// newLedgerRunID identifies one prompt run across the events it produces.
func newLedgerRunID() string {
	var raw [8]byte
	if _, err := crand.Read(raw[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(raw[:])
}
