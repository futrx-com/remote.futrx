package opencode

import (
	"encoding/json"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// Parser converts `opencode run --format json` newline-delimited JSON events
// into normalized agent events. Each line is one event:
//
//	{"type":"step_start","sessionID":"ses_…","part":{"type":"step-start",…}}
//	{"type":"text","sessionID":"ses_…","part":{"type":"text","text":"…"}}
//	{"type":"tool_use","sessionID":"ses_…","part":{"type":"tool","tool":"bash",
//	 "callID":"…","state":{"status":"completed","input":{…},"output":"…"}}}
//	{"type":"step_finish","sessionID":"ses_…","part":{"type":"step-finish",
//	 "reason":"stop","tokens":{"input":…,"output":…,"reasoning":…,
//	 "cache":{"read":…,"write":…}},"cost":0}}
//
// The CLI does not always emit a final step_finish after the last text part,
// so Provider.Run emits the terminal run.completed if the stream ended without
// one (see Completed). Token buckets are disjoint (input excludes cache
// activity), so no inclusive-input normalization is applied. The model is not
// reported on the wire; the requested model is used for attribution instead.
type Parser struct {
	req          agent.RunRequest
	sawSessionID string
	usage        agent.Usage
	completed    bool
}

func NewParser(req agent.RunRequest) *Parser {
	if req.Provider == "" {
		req.Provider = agent.ProviderOpenCode
	}
	return &Parser{req: req, sawSessionID: req.ResumeID}
}

type wireEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionID"`
	Part      struct {
		ID        string          `json:"id"`
		MessageID string          `json:"messageID"`
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		Tool      string          `json:"tool"`
		CallID    string          `json:"callID"`
		State     *wireToolState  `json:"state"`
		Reason    string          `json:"reason"`
		Tokens    *wireTokens     `json:"tokens"`
		Cost      *float64        `json:"cost"`
		Error     json.RawMessage `json:"error"`
	} `json:"part"`
}

type wireToolState struct {
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
	Output string          `json:"output"`
}

type wireTokens struct {
	Input     int64 `json:"input"`
	Output    int64 `json:"output"`
	Reasoning int64 `json:"reasoning"`
	Cache     struct {
		Read  int64 `json:"read"`
		Write int64 `json:"write"`
	} `json:"cache"`
}

func (p *Parser) ParseLine(line []byte) ([]agent.Event, error) {
	rawLine := append(json.RawMessage(nil), line...)
	var event wireEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	events := make([]agent.Event, 0, 2)

	if event.SessionID != "" && event.SessionID != p.sawSessionID {
		p.sawSessionID = event.SessionID
		events = append(events, p.event(now, agent.EventSessionUpdated, rawLine, func(ev *agent.Event) {
			ev.SessionID = event.SessionID
		}))
	}

	switch event.Type {
	case "text", "reasoning":
		events = append(events, p.parseText(now, event, rawLine)...)

	case "tool_use":
		events = append(events, p.parseToolUse(now, event, rawLine)...)

	case "step_finish":
		p.recordUsage(event)
		if event.Part.Reason == "stop" {
			p.completed = true
			events = append(events, p.completionEvent(now, rawLine))
		}

	case "error":
		message := event.Part.Text
		if message == "" {
			message = string(event.Part.Error)
		}
		if message == "" {
			message = string(rawLine)
		}
		events = append(events, p.event(now, agent.EventError, rawLine, func(ev *agent.Event) {
			ev.Message = message
		}))
	}

	return events, nil
}

func (p *Parser) parseText(now int64, event wireEvent, raw json.RawMessage) []agent.Event {
	if event.Part.Text == "" {
		return nil
	}
	if event.Type == "reasoning" {
		return []agent.Event{p.event(now, agent.EventReasoningDelta, raw, func(ev *agent.Event) {
			ev.ItemKind = agent.ItemReasoning
			ev.Text = event.Part.Text
		})}
	}
	return []agent.Event{p.event(now, agent.EventAssistantTextDelta, raw, func(ev *agent.Event) {
		ev.ItemKind = agent.ItemMessage
		ev.Text = event.Part.Text
	})}
}

func (p *Parser) parseToolUse(now int64, event wireEvent, raw json.RawMessage) []agent.Event {
	if event.Part.State == nil {
		return nil
	}
	toolName := event.Part.Tool
	callID := event.Part.CallID
	state := event.Part.State
	if state.Status == "completed" {
		return []agent.Event{p.event(now, agent.EventToolCompleted, raw, func(ev *agent.Event) {
			ev.ItemKind = agent.ItemToolCall
			ev.ItemID = callID
			ev.ToolName = toolName
			ev.Input = state.Input
			ev.Output = state.Output
		})}
	}
	return []agent.Event{p.event(now, agent.EventToolStarted, raw, func(ev *agent.Event) {
		ev.ItemKind = agent.ItemToolCall
		ev.ItemID = callID
		ev.ToolName = toolName
		ev.Input = state.Input
	})}
}

func (p *Parser) recordUsage(event wireEvent) {
	if event.Part.Tokens == nil {
		return
	}
	tokens := event.Part.Tokens
	p.usage = agent.Usage{
		InputTokens:      tokens.Input,
		OutputTokens:     tokens.Output,
		CacheReadTokens:  tokens.Cache.Read,
		CacheWriteTokens: tokens.Cache.Write,
		ReasoningTokens:  tokens.Reasoning,
		CostUSD:          event.Part.Cost,
		Model:            p.req.Model,
	}
}

func (p *Parser) completionEvent(now int64, raw json.RawMessage) agent.Event {
	return p.event(now, agent.EventRunCompleted, raw, func(ev *agent.Event) {
		ev.Usage = p.usage.Raw()
	})
}

// Completed reports whether the stream emitted a terminating step_finish.
func (p *Parser) Completed() bool {
	return p.completed
}

// RunUsage returns whatever the stream disclosed about this turn, falling back
// to the requested model so a run is still attributable when no token counts
// exist.
func (p *Parser) RunUsage() agent.Usage {
	usage := p.usage
	if usage.Model == "" {
		usage.Model = p.req.Model
	}
	return usage
}

func (p *Parser) event(now int64, type_ agent.EventType, raw json.RawMessage, fn func(*agent.Event)) agent.Event {
	ev := agent.Event{
		T:              now,
		Type:           type_,
		Provider:       agent.ProviderOpenCode,
		ConversationID: p.req.ConversationID,
		Raw:            raw,
	}
	if fn != nil {
		fn(&ev)
	}
	return ev
}
