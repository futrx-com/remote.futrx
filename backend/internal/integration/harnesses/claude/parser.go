package claude

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
)

// Parser converts Claude Code headless stream-json lines into normalized agent
// events. It owns Claude's native event shape; callers only see harness.Event.
type Parser struct {
	req                harness.RunRequest
	sawSessionID       string
	sawModel           string
	sawInputAcceptance bool
	messageUUID        string
	streamMessageID    string
}

func NewParser(req harness.RunRequest) *Parser {
	if req.Provider == "" {
		req.Provider = harness.ProviderClaude
	}
	return &Parser{req: req, sawSessionID: req.ResumeID, sawModel: req.Model}
}

func (p *Parser) ParseLine(line []byte) ([]harness.Event, error) {
	rawLine := append(json.RawMessage(nil), line...)
	var raw streamMsg
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, err
	}

	p.messageUUID = raw.UUID
	now := time.Now().UnixMilli()
	events := make([]harness.Event, 0, 2)
	if raw.Model != "" {
		p.sawModel = raw.Model
	}
	if raw.SessionID != "" && raw.SessionID != p.sawSessionID {
		p.sawSessionID = raw.SessionID
		events = append(events, p.event(now, harness.EventSessionUpdated, rawLine, func(ev *harness.Event) {
			ev.SessionID = raw.SessionID
			ev.Model = raw.Model
		}))
	}

	switch raw.Type {
	case "rate_limit_event":
		// The CLI volunteers one window per line, unprompted. Claude reports a
		// status and a reset time rather than a percentage, so a reading with
		// no number is normal and must not be stored as zero used.
		if quota, ok := parseRateLimit(raw.RateLimitInfo, now); ok {
			events = append(events, p.event(now, harness.EventQuotaUpdated, rawLine, func(ev *harness.Event) {
				ev.Quota = &quota
			}))
		}

	case "system":
		events = append(events, p.event(now, harness.EventSystem, rawLine, func(ev *harness.Event) {
			ev.Subtype = raw.Subtype
			ev.Data = raw.Message
			ev.ItemKind = harness.ItemSystem
		}))

	case "stream_event":
		var inner streamInner
		if err := json.Unmarshal(raw.Event, &inner); err != nil {
			return events, nil
		}
		if inner.Type == "message_start" {
			p.streamMessageID = inner.Message.ID
		} else if inner.Type == "message_stop" {
			p.streamMessageID = ""
		}
		if inner.Type == "content_block_delta" &&
			inner.Delta.Type == "text_delta" && inner.Delta.Text != "" {
			events = append(events, p.event(now, harness.EventAssistantTextDelta, rawLine, func(ev *harness.Event) {
				ev.ItemKind = harness.ItemMessage
				ev.Text = inner.Delta.Text
				// The wrapper UUID identifies this stream event, not its message.
				// Keep that UUID in Native.ItemID and use message_start's stable
				// API message identity for assembling text fragments.
				ev.MessageID = p.streamMessageID
			}))
		}

	case "assistant":
		if p.req.TrackInputAcceptance && !p.sawInputAcceptance && raw.UUID != "" && p.sawSessionID != "" {
			p.sawInputAcceptance = true
			events = append(events, p.event(now, harness.EventInputAccepted, rawLine, func(ev *harness.Event) {
				ev.SessionID = p.sawSessionID
				ev.Status = "claude_assistant_message"
			}))
		}
		var msg message
		if err := json.Unmarshal(raw.Message, &msg); err != nil {
			return events, nil
		}
		for _, block := range msg.Content {
			switch block.Type {
			case "tool_use":
				events = append(events, p.event(now, harness.EventToolStarted, rawLine, func(ev *harness.Event) {
					ev.ItemKind = harness.ItemToolCall
					ev.ItemID = block.ID
					ev.ToolName = block.Name
					ev.Input = block.Input
				}))
			case "thinking":
				if block.Text != "" {
					events = append(events, p.event(now, harness.EventReasoningDelta, rawLine, func(ev *harness.Event) {
						ev.ItemKind = harness.ItemReasoning
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
				events = append(events, p.event(now, harness.EventToolCompleted, rawLine, func(ev *harness.Event) {
					ev.ItemKind = harness.ItemToolCall
					ev.ItemID = block.ToolUseID
					ev.Output = normalizeToolResult(block.Content)
					ev.IsError = block.IsError
				}))
			}
		}

	case "result":
		if raw.IsError {
			message := strings.TrimSpace(raw.Result)
			if message == "" {
				message = "Claude returned an error"
			}
			events = append(events, p.event(now, harness.EventRunFailed, rawLine, func(ev *harness.Event) {
				ev.Message = message
				ev.IsError = true
				ev.Usage = p.normalizeUsage(raw).Raw()
			}))
			return events, nil
		}
		events = append(events, p.event(now, harness.EventRunCompleted, rawLine, func(ev *harness.Event) {
			ev.Usage = p.normalizeUsage(raw).Raw()
		}))
	}
	return events, nil
}

// normalizeUsage folds Claude's `result` message into the shared usage shape.
// Claude is the only provider that prices a turn itself: `total_cost_usd` is
// authoritative and must never be replaced by an estimate downstream.
func (p *Parser) normalizeUsage(raw streamMsg) harness.Usage {
	usage := harness.Usage{
		CostUSD:    raw.TotalCostUSD,
		DurationMs: raw.DurationMs,
		Turns:      raw.NumTurns,
		Model:      p.sawModel,
	}
	if parsed, ok := harness.ParseUsage(raw.Usage); ok {
		usage.InputTokens = parsed.InputTokens
		usage.OutputTokens = parsed.OutputTokens
		usage.CacheReadTokens = parsed.CacheReadTokens
		usage.CacheWriteTokens = parsed.CacheWriteTokens
	}
	return usage
}

func (p *Parser) event(
	now int64,
	type_ harness.EventType,
	raw json.RawMessage,
	fn func(*harness.Event),
) harness.Event {
	ev := harness.Event{
		T:              now,
		Type:           type_,
		Provider:       harness.ProviderClaude,
		ConversationID: p.req.ConversationID,
		Raw:            raw,
		MessageID:      p.messageUUID,
	}
	if p.req.TrackInputAcceptance {
		var envelope streamMsg
		_ = json.Unmarshal(raw, &envelope)
		ev.Native = &harness.NativeEnvelope{SchemaVersion: harness.NativeEnvelopeSchemaVersion, Method: envelope.Type, ThreadID: p.sawSessionID, ItemID: envelope.UUID, Payload: raw}
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
	UUID      string          `json:"uuid,omitempty"`
	Subtype   string          `json:"subtype,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Model     string          `json:"model,omitempty"`
	Message   json.RawMessage `json:"message,omitempty"`
	Event     json.RawMessage `json:"event,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Result    string          `json:"result,omitempty"`
	Usage     json.RawMessage `json:"usage,omitempty"`
	// RateLimitInfo rides on a top-level "rate_limit_event" line and carries
	// one subscription window.
	RateLimitInfo json.RawMessage `json:"rate_limit_info,omitempty"`
	// The `result` message prices and times the whole turn outside `usage`.
	TotalCostUSD *float64 `json:"total_cost_usd,omitempty"`
	DurationMs   int64    `json:"duration_ms,omitempty"`
	NumTurns     int64    `json:"num_turns,omitempty"`
}

type streamInner struct {
	Message struct {
		ID string `json:"id"`
	} `json:"message,omitempty"`
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
	IsError   bool            `json:"is_error,omitempty"`
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
