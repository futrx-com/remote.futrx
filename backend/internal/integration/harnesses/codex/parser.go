package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

// Parser converts `codex exec --json` JSONL events into normalized agent
// events. Codex emits whole item snapshots; this parser keeps per-item text
// state so the chat UI can continue to consume deltas.
type Parser struct {
	req          harness.RunRequest
	sawSessionID string
	itemText     map[string]string
}

func NewParser(req harness.RunRequest) *Parser {
	if req.Provider == "" {
		req.Provider = harness.ProviderCodex
	}
	return &Parser{
		req:          req,
		sawSessionID: req.ResumeID,
		itemText:     map[string]string{},
	}
}

func (p *Parser) ParseLine(line []byte) ([]harness.Event, error) {
	rawLine := append(json.RawMessage(nil), line...)
	var raw streamMsg
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	events := make([]harness.Event, 0, 3)

	switch raw.Type {
	case "thread.started":
		if raw.ThreadID != "" && raw.ThreadID != p.sawSessionID {
			p.sawSessionID = raw.ThreadID
			events = append(events, p.event(now, harness.EventSessionUpdated, rawLine, func(ev *harness.Event) {
				ev.SessionID = raw.ThreadID
			}))
		}

	case "item.started":
		events = append(events, p.itemStarted(now, rawLine, raw.Item)...)

	case "item.updated":
		events = append(events, p.itemUpdated(now, rawLine, raw.Item)...)

	case "item.completed":
		events = append(events, p.itemCompleted(now, rawLine, raw.Item)...)

	case "turn.completed":
		events = append(events, p.event(now, harness.EventRunCompleted, rawLine, func(ev *harness.Event) {
			ev.Usage = p.normalizeUsage(raw.Usage)
		}))

	case "turn.failed":
		message := strings.TrimSpace(raw.Error.Message)
		if message == "" {
			message = "Codex turn failed"
		}
		events = append(events, p.event(now, harness.EventRunFailed, rawLine, func(ev *harness.Event) {
			ev.Message = message
			ev.IsError = true
		}))

	case "error":
		message := strings.TrimSpace(raw.Message)
		if message == "" {
			message = strings.TrimSpace(raw.Error.Message)
		}
		if message == "" {
			message = "Codex stream error"
		}
		events = append(events, p.event(now, harness.EventError, rawLine, func(ev *harness.Event) {
			ev.Message = message
			ev.IsError = true
		}))
	}

	return events, nil
}

func (p *Parser) itemStarted(now int64, raw json.RawMessage, item codexItem) []harness.Event {
	switch item.Type {
	case "command_execution":
		return []harness.Event{p.toolStarted(now, raw, item.ID, "Bash", mustJSON(map[string]any{
			"command": item.Command,
		}))}
	case "mcp_tool_call":
		return []harness.Event{p.toolStarted(now, raw, item.ID, mcpToolName(item), item.Arguments)}
	case "collab_tool_call":
		return []harness.Event{p.toolStarted(now, raw, item.ID, "Collab:"+item.Tool, item.Raw)}
	case "web_search":
		return []harness.Event{p.toolStarted(now, raw, item.ID, "WebSearch", mustJSON(map[string]any{
			"query":  item.Query,
			"action": item.Action,
		}))}
	default:
		return nil
	}
}

func (p *Parser) itemUpdated(now int64, raw json.RawMessage, item codexItem) []harness.Event {
	switch item.Type {
	case "agent_message":
		return p.textDelta(now, raw, item, harness.EventAssistantTextDelta, harness.ItemMessage)
	case "reasoning":
		return p.textDelta(now, raw, item, harness.EventReasoningDelta, harness.ItemReasoning)
	default:
		return nil
	}
}

func (p *Parser) itemCompleted(now int64, raw json.RawMessage, item codexItem) []harness.Event {
	switch item.Type {
	case "agent_message":
		return p.textDelta(now, raw, item, harness.EventAssistantTextDelta, harness.ItemMessage)
	case "reasoning":
		return p.textDelta(now, raw, item, harness.EventReasoningDelta, harness.ItemReasoning)
	case "command_execution":
		return []harness.Event{p.toolCompleted(now, raw, item.ID, item.AggregatedOutput, toolFailed(item))}
	case "mcp_tool_call":
		return []harness.Event{p.toolCompleted(now, raw, item.ID, mcpOutput(item), item.Status == "failed" || item.Error.Message != "")}
	case "collab_tool_call":
		return []harness.Event{p.toolCompleted(now, raw, item.ID, collabOutput(item), item.Status == "failed")}
	case "file_change":
		input := mustJSON(map[string]any{"changes": item.Changes, "status": item.Status})
		return []harness.Event{
			p.toolStarted(now, raw, item.ID, "Patch", input),
			p.toolCompleted(now, raw, item.ID, item.Status, item.Status == "failed"),
		}
	case "web_search":
		return []harness.Event{p.toolCompleted(now, raw, item.ID, strings.TrimSpace(item.Query), false)}
	case "error":
		if item.Message != "" {
			return []harness.Event{p.event(now, harness.EventSystem, raw, func(ev *harness.Event) {
				ev.Subtype = "codex_warning"
				ev.Message = item.Message
				ev.ItemKind = harness.ItemSystem
			})}
		}
	}
	return nil
}

func (p *Parser) textDelta(
	now int64,
	raw json.RawMessage,
	item codexItem,
	eventType harness.EventType,
	kind harness.ItemKind,
) []harness.Event {
	text := item.Text
	if text == "" {
		return nil
	}
	id := item.ID
	if id == "" {
		id = string(kind)
	}
	prev := p.itemText[id]
	delta := text
	if strings.HasPrefix(text, prev) {
		delta = text[len(prev):]
	}
	p.itemText[id] = text
	if delta == "" {
		return nil
	}
	return []harness.Event{p.event(now, eventType, raw, func(ev *harness.Event) {
		ev.ItemID = item.ID
		ev.ItemKind = kind
		ev.Text = delta
	})}
}

func (p *Parser) toolStarted(
	now int64,
	raw json.RawMessage,
	id string,
	name string,
	input json.RawMessage,
) harness.Event {
	return p.event(now, harness.EventToolStarted, raw, func(ev *harness.Event) {
		ev.ItemKind = harness.ItemToolCall
		ev.ItemID = id
		ev.ToolName = strings.TrimSpace(name)
		if ev.ToolName == "" {
			ev.ToolName = "CodexTool"
		}
		ev.Input = input
	})
}

func (p *Parser) toolCompleted(
	now int64,
	raw json.RawMessage,
	id string,
	output string,
	isError bool,
) harness.Event {
	return p.event(now, harness.EventToolCompleted, raw, func(ev *harness.Event) {
		ev.ItemKind = harness.ItemToolCall
		ev.ItemID = id
		ev.Output = output
		ev.IsError = isError
	})
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
		Provider:       harness.ProviderCodex,
		ConversationID: p.req.ConversationID,
		Raw:            raw,
	}
	if fn != nil {
		fn(&ev)
	}
	return ev
}

type streamMsg struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id,omitempty"`
	Item     codexItem       `json:"item,omitempty"`
	Usage    json.RawMessage `json:"usage,omitempty"`
	Message  string          `json:"message,omitempty"`
	Error    struct {
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
}

type codexItem struct {
	ID               string          `json:"id,omitempty"`
	Type             string          `json:"type,omitempty"`
	Text             string          `json:"text,omitempty"`
	Command          string          `json:"command,omitempty"`
	AggregatedOutput string          `json:"aggregated_output,omitempty"`
	ExitCode         *int            `json:"exit_code,omitempty"`
	Status           string          `json:"status,omitempty"`
	Server           string          `json:"server,omitempty"`
	Tool             string          `json:"tool,omitempty"`
	Arguments        json.RawMessage `json:"arguments,omitempty"`
	Result           struct {
		Content           []json.RawMessage `json:"content,omitempty"`
		StructuredContent json.RawMessage   `json:"structured_content,omitempty"`
	} `json:"result,omitempty"`
	Error struct {
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
	Changes json.RawMessage `json:"changes,omitempty"`
	Query   string          `json:"query,omitempty"`
	Action  json.RawMessage `json:"action,omitempty"`
	Message string          `json:"message,omitempty"`
	Raw     json.RawMessage `json:"-"`
}

func (i *codexItem) UnmarshalJSON(data []byte) error {
	type alias codexItem
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*i = codexItem(a)
	i.Raw = append(json.RawMessage(nil), data...)
	return nil
}

func toolFailed(item codexItem) bool {
	if item.Status == "failed" || item.Status == "declined" {
		return true
	}
	return item.ExitCode != nil && *item.ExitCode != 0
}

func mcpToolName(item codexItem) string {
	if item.Server == "" {
		return item.Tool
	}
	if item.Tool == "" {
		return item.Server
	}
	return item.Server + "/" + item.Tool
}

func mcpOutput(item codexItem) string {
	if item.Error.Message != "" {
		return item.Error.Message
	}
	var out strings.Builder
	for _, raw := range item.Result.Content {
		var block struct {
			Type string `json:"type,omitempty"`
			Text string `json:"text,omitempty"`
		}
		if err := json.Unmarshal(raw, &block); err == nil && block.Text != "" {
			out.WriteString(block.Text)
			continue
		}
		if len(raw) > 0 {
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(string(raw))
		}
	}
	if out.Len() > 0 {
		return out.String()
	}
	if len(item.Result.StructuredContent) > 0 {
		return string(item.Result.StructuredContent)
	}
	return ""
}

func collabOutput(item codexItem) string {
	if item.Error.Message != "" {
		return item.Error.Message
	}
	if item.Status != "" {
		return item.Status
	}
	return ""
}

// normalizeUsage folds `turn.completed.usage` into the shared usage shape.
// ParseUsage converts Codex's inclusive input count into disjoint uncached,
// cache-read, and cache-write buckets. Codex reports no price, so cost stays
// unknown here and is estimated from the price table downstream.
func (p *Parser) normalizeUsage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	usage, ok := harness.ParseUsage(raw)
	if !ok {
		return raw
	}
	usage = harness.NormalizeInclusiveInput(usage)
	usage.Model = p.req.Model
	return usage.Raw()
}

func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error":%q}`, err.Error()))
	}
	return data
}
