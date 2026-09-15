package devinharness

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// acpEventParser translates ACP session/update notifications and
// server-to-client requests into normalized agent.Event values. It owns no
// run state; the run loop decides which events are terminal.
type acpEventParser struct {
	req           agent.RunRequest
	providerLabel string
	lastUsage     json.RawMessage
}

func newACPEventParser(req agent.RunRequest, providerLabel string) *acpEventParser {
	return &acpEventParser{
		req:           req,
		providerLabel: providerLabel,
	}
}

// ParseNotification translates one ACP notification (method + params) into zero
// or more agent events. It handles session/update notifications and filters
// Devin-specific _cognition.ai/* extension notifications.
func (parser *acpEventParser) ParseNotification(method string, raw json.RawMessage) []agent.Event {
	now := time.Now().UnixMilli()

	// Devin-specific extension notifications arrive on the same stdout stream.
	// They are NOT ACP-standard and must be filtered.
	if strings.HasPrefix(method, "_cognition.ai/") {
		return parser.handleCognitionExtension(now, method, raw)
	}

	switch method {
	case "session/update":
		return parser.parseSessionUpdate(now, raw)
	}

	// Unknown ACP-standard notification → native fallback.
	return parser.nativeEvent(now, method, raw)
}

// handleCognitionExtension filters Devin-specific _cognition.ai/* notifications.
// _cognition.ai/output is an MCP log line — consume silently (log to stderr).
// _cognition.ai/mcp/serversChanged and _cognition.ai/document/* are not
// relevant in headless mode — consume silently. Other _cognition.ai/* are
// forwarded as EventProviderNative.
func (parser *acpEventParser) handleCognitionExtension(now int64, method string, raw json.RawMessage) []agent.Event {
	switch method {
	case "_cognition.ai/output":
		log.Printf("%s[%s] cognition output: %s", parser.req.Provider, parser.req.ConversationID, truncateJSON(raw, 200))
		return nil
	case "_cognition.ai/mcp/serversChanged":
		return nil
	}
	if strings.HasPrefix(method, "_cognition.ai/document/") {
		return nil
	}
	return parser.nativeEvent(now, method, raw)
}

// parseSessionUpdate decodes the SessionUpdate union and dispatches to the
// variant handler matching the plan's "ACP → agent.Event mapping" table.
func (parser *acpEventParser) parseSessionUpdate(now int64, raw json.RawMessage) []agent.Event {
	var params acpSessionNotificationParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return parser.nativeEvent(now, "session/update", raw)
	}
	if len(params.Update) == 0 {
		return nil
	}

	var update acpSessionUpdate
	if err := json.Unmarshal(params.Update, &update); err != nil {
		return parser.nativeEvent(now, "session/update", raw)
	}

	switch update.SessionUpdate {
	case "agent_message_chunk":
		return parser.agentMessageChunk(now, update, raw)

	case "agent_thought_chunk":
		return parser.agentThoughtChunk(now, update, raw)

	case "user_message_chunk":
		// Echo of user input — consume silently, no event needed.
		return nil

	case "tool_call":
		return parser.toolCall(now, update, raw)

	case "tool_call_update":
		return parser.toolCallUpdate(now, update, raw)

	case "plan":
		return parser.nativeSessionUpdate(now, "plan", update, raw)

	case "available_commands_update":
		return parser.nativeSessionUpdate(now, "available_commands_update", update, raw)

	case "current_mode_update":
		return parser.nativeSessionUpdate(now, "current_mode_update", update, raw)

	case "config_option_update":
		return parser.nativeSessionUpdate(now, "config_option_update", update, raw)

	default:
		// Unknown sessionUpdate variant → native fallback with the variant
		// name as the method.
		return parser.nativeSessionUpdate(now, update.SessionUpdate, update, raw)
	}
}

func (parser *acpEventParser) agentMessageChunk(now int64, update acpSessionUpdate, raw json.RawMessage) []agent.Event {
	text := extractText(update.Content)
	if text == "" {
		return nil
	}
	return []agent.Event{parser.event(now, "session/update", agent.EventAssistantTextDelta, raw, func(event *agent.Event) {
		event.ItemKind = agent.ItemMessage
		event.Text = text
		event.MessageID = update.MessageID
	})}
}

func (parser *acpEventParser) agentThoughtChunk(now int64, update acpSessionUpdate, raw json.RawMessage) []agent.Event {
	text := extractText(update.Content)
	if text == "" {
		return nil
	}
	return []agent.Event{parser.event(now, "session/update", agent.EventReasoningDelta, raw, func(event *agent.Event) {
		event.Text = text
		event.MessageID = update.MessageID
	})}
}

func (parser *acpEventParser) toolCall(now int64, update acpSessionUpdate, raw json.RawMessage) []agent.Event {
	return []agent.Event{parser.event(now, "session/update", agent.EventToolStarted, raw, func(event *agent.Event) {
		event.ItemKind = agent.ItemToolCall
		event.ItemID = update.ToolCallID
		event.ToolName = update.Title
		event.Input = cloneRaw(update.RawInput)
	})}
}

func (parser *acpEventParser) toolCallUpdate(now int64, update acpSessionUpdate, raw json.RawMessage) []agent.Event {
	status := strings.ToLower(update.Status)
	switch status {
	case "completed", "failed", "cancelled", "denied":
		output := extractText(update.Content)
		if output == "" && len(update.RawOutput) > 0 {
			output = compactJSON(update.RawOutput)
		}
		return []agent.Event{parser.event(now, "session/update", agent.EventToolCompleted, raw, func(event *agent.Event) {
			event.ItemKind = agent.ItemToolCall
			event.ItemID = update.ToolCallID
			event.Output = output
			event.IsError = status == "failed"
		})}
	default:
		// Non-terminal progress update — consume silently.
		return nil
	}
}

// nativeSessionUpdate emits an EventProviderNative for a SessionUpdate variant
// that has no direct agent.Event mapping.
func (parser *acpEventParser) nativeSessionUpdate(now int64, method string, update acpSessionUpdate, raw json.RawMessage) []agent.Event {
	return []agent.Event{parser.event(now, "session/update", agent.EventProviderNative, raw, func(event *agent.Event) {
		event.Native = &agent.NativeEnvelope{
			SchemaVersion: agent.NativeEnvelopeSchemaVersion,
			Method:        method,
			Payload:       cloneRaw(raw),
		}
	})}
}

func (parser *acpEventParser) nativeEvent(now int64, method string, raw json.RawMessage) []agent.Event {
	return []agent.Event{parser.event(now, method, agent.EventProviderNative, raw, func(event *agent.Event) {
		event.Native = &agent.NativeEnvelope{
			SchemaVersion: agent.NativeEnvelopeSchemaVersion,
			Method:        method,
			Payload:       cloneRaw(raw),
		}
	})}
}

func (parser *acpEventParser) event(now int64, method string, eventType agent.EventType, raw json.RawMessage, configure func(*agent.Event)) agent.Event {
	event := agent.Event{
		T:              now,
		Type:           eventType,
		Provider:       parser.req.Provider,
		ConversationID: parser.req.ConversationID,
		Raw:            cloneRaw(raw),
	}
	if configure != nil {
		configure(&event)
	}
	return event
}

// extractText pulls the text field from a ContentBlock JSON blob.
func extractText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var block acpContentBlock
	if json.Unmarshal(content, &block) == nil && block.Type == "text" {
		return block.Text
	}
	// content may be an array of blocks
	var blocks []acpContentBlock
	if json.Unmarshal(content, &blocks) == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Type == "text" {
				sb.WriteString(b.Text)
			}
		}
		return sb.String()
	}
	return ""
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return string(raw)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return string(raw)
	}
	return string(data)
}

func truncateJSON(raw json.RawMessage, limit int) string {
	s := string(raw)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
