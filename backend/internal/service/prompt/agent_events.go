package prompt

import (
	"context"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
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

	if ev.Type == agent.EventToolStarted && rnr.observer != nil {
		rnr.observer.RunToolStarted(ctx, id, ev.ToolName)
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

	out := ChatEvent{T: t}
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
	case agent.EventReasoningDelta:
		out.Type = "thinking"
		out.Text = ev.Text
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
	case agent.EventRunCompleted:
		out.Type = "complete"
		out.Usage = ev.Usage
	case agent.EventRunFailed, agent.EventError:
		out.Type = "error"
		out.Message = ev.Message
	default:
		return ChatEvent{}, false
	}
	return out, true
}
