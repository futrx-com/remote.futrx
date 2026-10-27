package kimi

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// Parser converts @moonshot-ai/kimi-code `kimi -p --output-format stream-json`
// JSONL into normalized agent events. Unlike Claude/Codex, kimi-code emits
// OpenAI-chat-shaped lines: {"role":"assistant",...}, {"role":"tool",...}, and
// a trailing {"role":"meta","type":"session.resume_hint",...}. As of
// kimi-code v0.19.2 there is no usage/token line and no thinking line; the
// meta line marks completion. Token counts are therefore unknown for kimi
// runs — the parser still forwards a `usage` object opportunistically if a
// future CLI version starts emitting one on any line.
type Parser struct {
	req          agent.RunRequest
	sawSessionID string
	sawUsage     agent.Usage
}

func NewParser(req agent.RunRequest) *Parser {
	if req.Provider == "" {
		req.Provider = agent.ProviderKimi
	}
	return &Parser{req: req, sawSessionID: req.ResumeID}
}

type wireMsg struct {
	Role       string          `json:"role"`
	Type       string          `json:"type,omitempty"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  []wireToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	SessionID  string          `json:"session_id,omitempty"`
	Model      string          `json:"model,omitempty"`
	Usage      json.RawMessage `json:"usage,omitempty"`
}

type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func (p *Parser) ParseLine(line []byte) ([]agent.Event, error) {
	rawLine := append(json.RawMessage(nil), line...)
	var raw wireMsg
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	events := make([]agent.Event, 0, 2)

	if usage, ok := agent.ParseUsage(raw.Usage); ok {
		p.sawUsage = usage
	}
	if raw.Model != "" {
		p.sawUsage.Model = raw.Model
	}

	switch raw.Role {
	case "meta":
		// Final line: {"role":"meta","type":"session.resume_hint","session_id":...}.
		if raw.Type == "session.resume_hint" {
			if raw.SessionID != "" && raw.SessionID != p.sawSessionID {
				p.sawSessionID = raw.SessionID
				events = append(events, p.event(now, agent.EventSessionUpdated, rawLine, func(ev *agent.Event) {
					ev.SessionID = raw.SessionID
				}))
			}
			// kimi-code emits no turn-completed line; the resume hint is the
			// de-facto end of the run. Usage is normally absent, so the event
			// carries the model alone and cost stays unknown downstream.
			usage := p.runUsage()
			events = append(events, p.event(now, agent.EventRunCompleted, rawLine, func(ev *agent.Event) {
				ev.Usage = usage.Raw()
			}))
		}

	case "assistant":
		if text := decodeContent(raw.Content); text != "" {
			events = append(events, p.event(now, agent.EventAssistantTextDelta, rawLine, func(ev *agent.Event) {
				ev.ItemKind = agent.ItemMessage
				ev.Text = text
			}))
		}
		for _, tc := range raw.ToolCalls {
			tc := tc
			events = append(events, p.event(now, agent.EventToolStarted, rawLine, func(ev *agent.Event) {
				ev.ItemKind = agent.ItemToolCall
				ev.ItemID = tc.ID
				ev.ToolName = strings.TrimSpace(tc.Function.Name)
				if ev.ToolName == "" {
					ev.ToolName = "KimiTool"
				}
				// kimi encodes arguments as a JSON-encoded string; it is already
				// valid JSON, so pass the bytes straight through as tool input.
				if args := strings.TrimSpace(tc.Function.Arguments); args != "" {
					ev.Input = json.RawMessage(args)
				}
			}))
		}

	case "tool":
		events = append(events, p.event(now, agent.EventToolCompleted, rawLine, func(ev *agent.Event) {
			ev.ItemKind = agent.ItemToolCall
			ev.ItemID = raw.ToolCallID
			ev.Output = decodeContent(raw.Content)
		}))
	}

	return events, nil
}

// runUsage returns whatever the stream disclosed about this turn, falling
// back to the requested model so a kimi run is still attributable even when
// no token counts exist.
func (p *Parser) runUsage() agent.Usage {
	usage := p.sawUsage
	if usage.Model == "" {
		usage.Model = p.req.Model
	}
	return usage
}

func (p *Parser) event(now int64, type_ agent.EventType, raw json.RawMessage, fn func(*agent.Event)) agent.Event {
	ev := agent.Event{
		T:              now,
		Type:           type_,
		Provider:       agent.ProviderKimi,
		ConversationID: p.req.ConversationID,
		Raw:            raw,
	}
	if fn != nil {
		fn(&ev)
	}
	return ev
}

// decodeContent renders a kimi-code message `content` field as text. It is
// usually a JSON string but tolerates a content-block array shape.
func decodeContent(raw json.RawMessage) string {
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
		for _, b := range blocks {
			out.WriteString(b.Text)
		}
		return out.String()
	}
	return string(raw)
}
