package kimi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func (r *serverRun) onEvent(raw json.RawMessage) error {
	var frame serverEvent
	if err := json.Unmarshal(raw, &frame); err != nil {
		return fmt.Errorf("decode Kimi event: %w", err)
	}
	if frame.Type == "resync_required" {
		return fmt.Errorf("Kimi requires event resynchronization; resume the session to recover")
	}
	if frame.SessionID != r.session {
		return nil
	}
	// Durable events can be replayed after subscribing. Volatile deltas share
	// sequence numbers with durable events and must not be deduplicated this way.
	if !frame.Volatile && frame.Seq > 0 {
		if frame.Epoch == r.epoch && frame.Seq <= r.seq {
			return nil
		}
		r.seq = frame.Seq
		r.epoch = frame.Epoch
	}
	var p nativePayload
	if err := json.Unmarshal(frame.Payload, &p); err != nil {
		return err
	}
	kind := strings.TrimPrefix(frame.Type, "event.")
	if p.Type != "" {
		kind = strings.TrimPrefix(p.Type, "event.")
	}
	who := p.AgentID
	if who == "" {
		who = "main"
	}
	native := &agent.NativeEnvelope{SchemaVersion: agent.NativeEnvelopeSchemaVersion, Method: "kimi/" + kind, ThreadID: r.session + ":" + who, TurnID: fmt.Sprint(p.TurnID), ItemID: p.ToolCallID, Payload: frame.Payload}
	ev := agent.Event{Type: agent.EventProviderNative, Native: native, Data: frame.Payload}
	if strings.HasPrefix(kind, "approval.") || strings.HasPrefix(kind, "question.") {
		return r.interactionEvent(kind, frame.Payload)
	}
	child := who != "main"
	switch kind {
	case "task.started", "task.terminated":
		if c := r.activity.task(kind, who, frame.Payload); c != nil {
			r.publishChild(c, native)
		}
	case "context.spliced":
		// Context snapshots may repeat private interaction answers and inline media.
		// Streaming text/tools already carry the user-visible transcript.
		native.Payload = nil
		ev.Data = nil
	case "cron.fired":
		r.cron.fired(frame.Payload)
		r.mainEnded = false
	case "compaction.started":
		if !child {
			r.compacting = true
			ev.Type = agent.EventTurnStatus
			ev.Status = "compacting"
		}
	case "compaction.completed":
		if !child {
			r.compacting = false
			r.mainEnded = true
			ev.Type = agent.EventTurnStatus
			ev.Status = "waiting"
		}
	case "compaction.cancelled", "compaction.blocked":
		if !child {
			r.compacting = false
			r.interrupted = true
		}
	case "turn.started":
		if !child {
			r.mainEnded = false
			r.usage.startTurn()
			ev.Type = agent.EventTurnStatus
			ev.Status = "running"
		}
	case "turn.ended":
		if child && r.activity.isSideConversation(who) {
			c, failure, interrupted := r.activity.endSideConversation(who, p)
			if failure != "" {
				r.failure = failure
			}
			if interrupted {
				r.interrupted = true
			}
			r.publishChild(c, native)
			return nil
		}
		if !child {
			r.mainEnded = true
			switch p.Reason {
			case "failed", "blocked":
				r.failure = nativeText(p.Error)
				if r.failure == "" {
					r.failure = "Kimi turn " + p.Reason
				}
			case "cancelled":
				r.interrupted = true
			}
			ev.Type = agent.EventTurnStatus
			ev.Status = p.Reason
			if p.Reason == "completed" {
				ev.Status = "waiting"
			}
		}
	case "turn.step.completed":
		if r.usage.recordStep(who, p) {
			ev.Type = agent.EventUsageUpdated
			ev.Usage = r.usage.raw()
		}
	case "assistant.delta", "thinking.delta":
		if child {
			r.publishChild(r.activity.delta(kind, who, p.Delta), native)
			return nil
		}
		ev.Type = agent.EventAssistantTextDelta
		if kind == "thinking.delta" {
			ev.Type = agent.EventReasoningDelta
		}
		ev.Text = p.Delta
		ev.MessageID = fmt.Sprintf("%s:%d:%s", r.session, p.TurnID, kind)
	case "tool.call.started":
		t := r.activity.startTool(r.session, who, p)
		if child {
			r.publishChild(r.activity.child(who), native)
			return nil
		}
		ev.Type = agent.EventToolStarted
		ev.ItemID = t.ID
		ev.ToolName = p.Name
		ev.Input = p.Args
	case "tool.result":
		id, t := r.activity.finishTool(r.session, who, p)
		if t != nil && !child {
			r.cron.toolResult(t)
		}
		if child {
			r.publishChild(r.activity.child(who), native)
			return nil
		}
		ev.Type = agent.EventToolCompleted
		ev.ItemID = id
		ev.Output = nativeText(p.Output)
		ev.IsError = p.IsError
	case "subagent.spawned":
		r.publishChild(r.activity.spawn(p), native)
		return nil
	case "subagent.started", "subagent.suspended", "subagent.completed", "subagent.failed":
		r.publishChild(r.activity.lifecycle(kind, p), native)
		return nil
	}
	r.publish(ev)
	return nil
}
