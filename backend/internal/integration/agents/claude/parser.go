package claude

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// Parser converts Claude Code headless stream-json lines into normalized agent
// events. It owns Claude's native event shape; callers only see agent.Event.
type Parser struct {
	req          agent.RunRequest
	sawSessionID string
	sawModel     string
}

func NewParser(req agent.RunRequest) *Parser {
	if req.Provider == "" {
		req.Provider = agent.ProviderClaude
	}
	return &Parser{req: req, sawSessionID: req.ResumeID, sawModel: req.Model}
}

func (p *Parser) ParseLine(line []byte) ([]agent.Event, error) {
	rawLine := append(json.RawMessage(nil), line...)
	var raw streamMsg
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	events := make([]agent.Event, 0, 2)
	if raw.Model != "" {
		p.sawModel = raw.Model
	}
	if raw.SessionID != "" && raw.SessionID != p.sawSessionID {
		p.sawSessionID = raw.SessionID
		events = append(events, p.event(now, agent.EventSessionUpdated, rawLine, func(ev *agent.Event) {
			ev.SessionID = raw.SessionID
			ev.Model = raw.Model
		}))
	}

	switch raw.Type {
	case "system":
		events = append(events, p.event(now, agent.EventSystem, rawLine, func(ev *agent.Event) {
			ev.Subtype = raw.Subtype
			ev.Data = raw.Message
			ev.ItemKind = agent.ItemSystem
		}))

	case "stream_event":
		var inner streamInner
		if err := json.Unmarshal(raw.Event, &inner); err != nil {
			return events, nil
		}
		if inner.Type == "content_block_delta" &&
			inner.Delta.Type == "text_delta" && inner.Delta.Text != "" {
			events = append(events, p.event(now, agent.EventAssistantTextDelta, rawLine, func(ev *agent.Event) {
				ev.ItemKind = agent.ItemMessage
				ev.Text = inner.Delta.Text
			}))
		}

	case "assistant":
		var msg message
		if err := json.Unmarshal(raw.Message, &msg); err != nil {
			return events, nil
		}
		for _, block := range msg.Content {
			switch block.Type {
			case "tool_use":
				events = append(events, p.event(now, agent.EventToolStarted, rawLine, func(ev *agent.Event) {
					ev.ItemKind = agent.ItemToolCall
					ev.ItemID = block.ID
					ev.ToolName = block.Name
					ev.Input = block.Input
				}))
			case "thinking":
				if block.Text != "" {
					events = append(events, p.event(now, agent.EventReasoningDelta, rawLine, func(ev *agent.Event) {
						ev.ItemKind = agent.ItemReasoning
						ev.Text = block.Text
					}))
				}
			}
		}

	case "user":
		var msg message
		if err := json.Unmarshal(raw.Message, &msg); err != nil {
			return events, nil
		}
		for _, block := range msg.Content {
			if block.Type == "tool_result" {
				events = append(events, p.event(now, agent.EventToolCompleted, rawLine, func(ev *agent.Event) {
					ev.ItemKind = agent.ItemToolCall
					ev.ItemID = block.ToolUseID
					ev.Output = normalizeToolResult(block.Content)
					ev.IsError = false
				}))
			}
		}

	case "result":
		if raw.IsError {
			message := strings.TrimSpace(raw.Result)
			if message == "" {
				message = "Claude returned an error"
			}
			events = append(events, p.event(now, agent.EventRunFailed, rawLine, func(ev *agent.Event) {
				ev.Message = message
				ev.IsError = true
				ev.Usage = p.normalizeUsage(raw).Raw()
			}))
			return events, nil
		}
		events = append(events, p.event(now, agent.EventRunCompleted, rawLine, func(ev *agent.Event) {
			ev.Usage = p.normalizeUsage(raw).Raw()
		}))
	}
	return events, nil
}

// normalizeUsage folds Claude's `result` message into the shared usage shape.
// Claude is the only provider that prices a turn itself: `total_cost_usd` is
// authoritative and must never be replaced by an estimate downstream.
func (p *Parser) normalizeUsage(raw streamMsg) agent.Usage {
	usage := agent.Usage{
		CostUSD:    raw.TotalCostUSD,
		DurationMs: raw.DurationMs,
		Turns:      raw.NumTurns,
		Model:      p.sawModel,
	}
	if parsed, ok := agent.ParseUsage(raw.Usage); ok {
		usage.InputTokens = parsed.InputTokens
		usage.OutputTokens = parsed.OutputTokens
		usage.CacheReadTokens = parsed.CacheReadTokens
		usage.CacheWriteTokens = parsed.CacheWriteTokens
	}
	return usage
}

func (p *Parser) event(
	now int64,
	type_ agent.EventType,
	raw json.RawMessage,
	fn func(*agent.Event),
) agent.Event {
	ev := agent.Event{
		T:              now,
		Type:           type_,
		Provider:       agent.ProviderClaude,
		ConversationID: p.req.ConversationID,
		Raw:            raw,
	}
	if fn != nil {
		fn(&ev)
	}
	return ev
}

// streamMsg is the on-wire JSON shape from `claude -p --output-format
// stream-json --include-partial-messages --verbose`. We parse the relevant
// subset; unknown fields are allowed.
type streamMsg struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Model     string          `json:"model,omitempty"`
	Message   json.RawMessage `json:"message,omitempty"`
	Event     json.RawMessage `json:"event,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Result    string          `json:"result,omitempty"`
	Usage     json.RawMessage `json:"usage,omitempty"`
	// The `result` message prices and times the whole turn outside `usage`.
	TotalCostUSD *float64 `json:"total_cost_usd,omitempty"`
	DurationMs   int64    `json:"duration_ms,omitempty"`
	NumTurns     int64    `json:"num_turns,omitempty"`
}

type streamInner struct {
	Type  string `json:"type"`
	Index int    `json:"index,omitempty"`
	Delta struct {
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"delta,omitempty"`
	ContentBlock struct {
		Type  string          `json:"type,omitempty"`
		ID    string          `json:"id,omitempty"`
		Name  string          `json:"name,omitempty"`
		Input json.RawMessage `json:"input,omitempty"`
	} `json:"content_block,omitempty"`
}

type message struct {
	ID      string         `json:"id,omitempty"`
	Role    string         `json:"role,omitempty"`
	Content []contentBlock `json:"content,omitempty"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

func normalizeToolResult(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var out strings.Builder
		for _, block := range blocks {
			if block.Type == "text" {
				out.WriteString(block.Text)
			}
		}
		return out.String()
	}
	return string(raw)
}
