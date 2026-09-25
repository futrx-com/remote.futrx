package prompt

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// runEventRelay keeps a provider run's private notification trailer out of
// persisted chat events while forwarding visible text and accounting events.
// It attributes events to the selected provider once, before any consumer
// sees them.
type runEventRelay struct {
	service       *Service
	ctx           context.Context
	chatID        servicechat.ID
	providerID    agent.ProviderID
	ledger        ledgerRun
	emit          func(ChatEvent)
	notification  notificationSummaryFilter
	terminalSeen  bool
	lastMessageID string
}

func (r *runEventRelay) forward(ev agent.Event) {
	ev = withDefaultProvider(ev, r.providerID)
	if ev.Type == agent.EventAssistantTextDelta {
		r.lastMessageID = agentEventMessageID(ev)
		ev.Text = r.notification.text(ev.Text)
		if ev.Text == "" {
			return
		}
	}
	if ev.Type == agent.EventRunCompleted || ev.Type == agent.EventRunFailed || ev.Type == agent.EventError {
		r.terminalSeen = true
		visible, summary := r.notification.finish()
		if visible != "" {
			r.service.emitAgentEvent(r.ctx, r.chatID, agent.Event{
				T: ev.T, Type: agent.EventAssistantTextDelta, Text: visible,
				Provider: ev.Provider, MessageID: r.lastMessageID,
			}, r.emit)
		}
		if ev.Type == agent.EventRunCompleted {
			ev.NotificationSummary = summary
		}
	}
	r.service.emitAgentEvent(r.ctx, r.chatID, ev, r.emit)
	r.service.recordRunUsage(r.ctx, r.ledger, ev)
	r.service.recordQuota(r.ctx, ev)
}

func (r *runEventRelay) finish() {
	if r.terminalSeen {
		return
	}
	visible, _ := r.notification.finish()
	if visible != "" {
		r.service.emitAgentEvent(r.ctx, r.chatID, agent.Event{
			Type: agent.EventAssistantTextDelta, Text: visible,
			Provider: r.providerID, MessageID: r.lastMessageID,
		}, r.emit)
	}
}
