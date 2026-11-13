package prompt

import (
	"context"
	"encoding/json"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// runEventRelay keeps a provider run's private notification trailer out of
// persisted chat events while forwarding visible text and accounting events.
// The trailer is stripped from assistant text and from the subagent messages
// collaboration events carry. It attributes events to the selected provider
// once, before any consumer sees them.
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
	if ev.Type == agent.EventCollaboration {
		ev.Data = stripSubagentNotificationSummaries(ev.Data)
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

// stripSubagentNotificationSummaries removes the private notification trailer
// from every subagent message in a collaboration event's data. Messages are
// decoded before matching because JSON encoders may escape the marker — Go's
// json.Marshal writes it as \u003cnotification_summary\u003e. Everything
// besides the stripped messages keeps its original bytes, so untouched
// payloads come back unchanged.
func stripSubagentNotificationSummaries(data json.RawMessage) json.RawMessage {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return data
	}
	rawStates, ok := payload["agentsStates"]
	if !ok {
		return data
	}
	var states map[string]json.RawMessage
	if err := json.Unmarshal(rawStates, &states); err != nil {
		return data
	}
	changed := false
	for threadID, rawState := range states {
		var state map[string]json.RawMessage
		if err := json.Unmarshal(rawState, &state); err != nil {
			continue
		}
		var message string
		if err := json.Unmarshal(state["message"], &message); err != nil {
			continue
		}
		visible := stripNotificationSummary(message)
		if visible == message {
			continue
		}
		encoded, err := json.Marshal(visible)
		if err != nil {
			continue
		}
		state["message"] = encoded
		if states[threadID], err = json.Marshal(state); err != nil {
			continue
		}
		changed = true
	}
	if !changed {
		return data
	}
	encodedStates, err := json.Marshal(states)
	if err != nil {
		return data
	}
	payload["agentsStates"] = encodedStates
	encoded, err := json.Marshal(payload)
	if err != nil {
		return data
	}
	return encoded
}
