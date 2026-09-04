package codexharness

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type appServerEventParser struct {
	req           agent.RunRequest
	providerLabel string
	itemText      map[string]string
	lastUsage     json.RawMessage
}

type appServerNativeIDs struct {
	ThreadID  string
	TurnID    string
	ItemID    string
	RequestID string
}

func newAppServerEventParser(req agent.RunRequest, providerLabel string) *appServerEventParser {
	return &appServerEventParser{
		req:           req,
		providerLabel: providerLabel,
		itemText:      make(map[string]string),
	}
}

func (parser *appServerEventParser) ParseNotification(method string, raw json.RawMessage) []agent.Event {
	now := time.Now().UnixMilli()
	switch method {
	case "item/agentMessage/delta", "item/plan/delta":
		var params appServerDeltaParams
		if json.Unmarshal(raw, &params) != nil || params.Delta == "" {
			return nil
		}
		parser.itemText[params.ItemID] += params.Delta
		return []agent.Event{parser.event(now, method, agent.EventAssistantTextDelta, raw, func(event *agent.Event) {
			event.ItemID = params.ItemID
			event.ItemKind = agent.ItemMessage
			event.Text = params.Delta
		})}

	case "item/reasoning/summaryTextDelta", "item/reasoning/textDelta":
		var params appServerDeltaParams
		if json.Unmarshal(raw, &params) != nil || params.Delta == "" {
			return nil
		}
		return []agent.Event{parser.event(now, method, agent.EventReasoningDelta, raw, func(event *agent.Event) {
			event.ItemID = params.ItemID
			event.ItemKind = agent.ItemReasoning
			event.Text = params.Delta
		})}

	case "item/started", "item/completed":
		var params appServerItemParams
		if json.Unmarshal(raw, &params) != nil {
			return parser.nativeEvent(now, method, raw)
		}
		if method == "item/started" {
			return parser.itemStarted(now, method, raw, params.Item)
		}
		return parser.itemCompleted(now, method, raw, params.Item)

	case "thread/tokenUsage/updated":
		var params appServerTokenUsageParams
		if json.Unmarshal(raw, &params) == nil {
			parser.lastUsage = params.TokenUsage.Last.normalized(parser.req.Model)
		}
		return []agent.Event{parser.event(now, method, agent.EventUsageUpdated, raw, func(event *agent.Event) {
			event.Usage = cloneRaw(parser.lastUsage)
		})}

	case "thread/status/changed":
		var params appServerThreadStatusParams
		if json.Unmarshal(raw, &params) != nil {
			return parser.nativeEvent(now, method, raw)
		}
		return []agent.Event{parser.event(now, method, agent.EventTurnStatus, raw, func(event *agent.Event) {
			event.Status = params.Status.Type
			if params.Status.Type == "active" && len(params.Status.ActiveFlags) > 0 {
				event.Status = strings.Join(params.Status.ActiveFlags, ",")
			}
			event.Data = cloneRaw(raw)
		})}

	case "turn/started":
		return []agent.Event{parser.event(now, method, agent.EventTurnStatus, raw, func(event *agent.Event) {
			event.Status = "inProgress"
			event.Data = cloneRaw(raw)
		})}

	case "turn/completed":
		return parser.turnCompleted(now, method, raw)

	case "serverRequest/resolved":
		// The request handler owns the typed lifecycle because it can distinguish
		// user answers from server cancellation and auto-resolution. Keep the
		// provider notification visible as a native fallback as well.
		return parser.nativeEvent(now, method, raw)

	case "error":
		var params appServerErrorParams
		if json.Unmarshal(raw, &params) == nil && strings.TrimSpace(params.Error.Message) != "" {
			return []agent.Event{parser.event(now, method, agent.EventError, raw, func(event *agent.Event) {
				event.Message = strings.TrimSpace(params.Error.Message)
				event.IsError = true
				if params.WillRetry {
					event.Status = "retrying"
				} else {
					event.Status = "terminal"
				}
			})}
		}
	}
	return parser.nativeEvent(now, method, raw)
}

func (parser *appServerEventParser) turnCompleted(now int64, method string, raw json.RawMessage) []agent.Event {
	var params appServerTurnCompletedParams
	if json.Unmarshal(raw, &params) != nil {
		return parser.nativeEvent(now, method, raw)
	}
	status := strings.TrimSpace(params.Turn.Status)
	switch status {
	case "completed":
		return []agent.Event{parser.event(now, method, agent.EventRunCompleted, raw, func(event *agent.Event) {
			event.Status = status
			event.Usage = cloneRaw(parser.lastUsage)
		})}
	case "interrupted":
		return []agent.Event{parser.event(now, method, agent.EventRunInterrupted, raw, func(event *agent.Event) {
			event.Status = status
			event.Usage = cloneRaw(parser.lastUsage)
		})}
	case "failed":
		message := parser.providerLabel + " turn failed"
		if params.Turn.Error != nil && strings.TrimSpace(params.Turn.Error.Message) != "" {
			message = strings.TrimSpace(params.Turn.Error.Message)
		}
		return []agent.Event{parser.event(now, method, agent.EventRunFailed, raw, func(event *agent.Event) {
			event.Status = status
			event.Message = message
			event.IsError = true
			event.Usage = cloneRaw(parser.lastUsage)
		})}
	default:
		return []agent.Event{parser.event(now, method, agent.EventRunFailed, raw, func(event *agent.Event) {
			event.Status = status
			event.Message = parser.providerLabel + " turn ended with unknown status: " + status
			event.IsError = true
			event.Usage = cloneRaw(parser.lastUsage)
		})}
	}
}

func (parser *appServerEventParser) itemStarted(
	now int64,
	method string,
	raw json.RawMessage,
	item appServerItem,
) []agent.Event {
	switch item.Type {
	case "commandExecution":
		return []agent.Event{parser.toolStarted(now, method, raw, item.ID, "Bash", mustJSON(map[string]any{
			"command": item.Command,
		}))}
	case "fileChange":
		return []agent.Event{parser.toolStarted(now, method, raw, item.ID, "Patch", mustJSON(map[string]any{
			"changes": item.Changes,
		}))}
	case "mcpToolCall":
		return []agent.Event{parser.toolStarted(now, method, raw, item.ID, appServerMCPToolName(item), item.Arguments)}
	case "dynamicToolCall":
		return []agent.Event{parser.toolStarted(now, method, raw, item.ID, joinedToolName(item.Namespace, item.Tool), item.Arguments)}
	case "collabAgentToolCall":
		if !projectCollaboration(item) {
			return parser.nativeEvent(now, method, raw)
		}
		return []agent.Event{parser.collaboration(now, method, raw, item)}
	case "webSearch":
		return []agent.Event{parser.toolStarted(now, method, raw, item.ID, "WebSearch", mustJSON(map[string]any{
			"query": item.Query, "action": item.Action,
		}))}
	default:
		return parser.nativeEvent(now, method, raw)
	}
}

func (parser *appServerEventParser) itemCompleted(
	now int64,
	method string,
	raw json.RawMessage,
	item appServerItem,
) []agent.Event {
	switch item.Type {
	case "agentMessage", "plan":
		return parser.completedText(now, method, raw, item)
	case "commandExecution":
		return []agent.Event{parser.toolCompleted(
			now, method, raw, item.ID, item.AggregatedOutput, appServerItemFailed(item),
		)}
	case "fileChange":
		return []agent.Event{parser.toolCompleted(now, method, raw, item.ID, item.Status, appServerItemFailed(item))}
	case "mcpToolCall", "dynamicToolCall":
		return []agent.Event{parser.toolCompleted(now, method, raw, item.ID, appServerToolOutput(item), appServerItemFailed(item))}
	case "collabAgentToolCall":
		if !projectCollaboration(item) {
			return parser.nativeEvent(now, method, raw)
		}
		return []agent.Event{parser.collaboration(now, method, raw, item)}
	case "webSearch":
		return []agent.Event{parser.toolCompleted(now, method, raw, item.ID, item.Query, false)}
	default:
		return parser.nativeEvent(now, method, raw)
	}
}

func projectCollaboration(item appServerItem) bool {
	return strings.TrimSpace(item.Tool) != "wait" ||
		len(item.ReceiverThreadIDs) > 0 || len(item.AgentsStates) > 0 || item.Prompt != nil
}

func (parser *appServerEventParser) completedText(
	now int64,
	method string,
	raw json.RawMessage,
	item appServerItem,
) []agent.Event {
	if item.Text == "" {
		return parser.nativeEvent(now, method, raw)
	}
	seen := parser.itemText[item.ID]
	delta := item.Text
	if strings.HasPrefix(item.Text, seen) {
		delta = item.Text[len(seen):]
	}
	parser.itemText[item.ID] = item.Text
	if delta == "" {
		return parser.nativeEvent(now, method, raw)
	}
	return []agent.Event{parser.event(now, method, agent.EventAssistantTextDelta, raw, func(event *agent.Event) {
		event.ItemID = item.ID
		event.ItemKind = agent.ItemMessage
		event.Text = delta
	})}
}

func (parser *appServerEventParser) collaboration(
	now int64,
	method string,
	raw json.RawMessage,
	item appServerItem,
) agent.Event {
	return parser.event(now, method, agent.EventCollaboration, raw, func(event *agent.Event) {
		event.ItemID = item.ID
		event.ItemKind = agent.ItemToolCall
		event.ToolName = strings.TrimSpace(item.Tool)
		event.Status = strings.TrimSpace(item.Status)
		event.Data = cloneRaw(item.Raw)
	})
}

func (parser *appServerEventParser) toolStarted(
	now int64,
	method string,
	raw json.RawMessage,
	id, name string,
	input json.RawMessage,
) agent.Event {
	return parser.event(now, method, agent.EventToolStarted, raw, func(event *agent.Event) {
		event.ItemID = id
		event.ItemKind = agent.ItemToolCall
		event.ToolName = strings.TrimSpace(name)
		if event.ToolName == "" {
			event.ToolName = parser.providerLabel + "Tool"
		}
		event.Input = cloneRaw(input)
	})
}

func (parser *appServerEventParser) toolCompleted(
	now int64,
	method string,
	raw json.RawMessage,
	id, output string,
	isError bool,
) agent.Event {
	return parser.event(now, method, agent.EventToolCompleted, raw, func(event *agent.Event) {
		event.ItemID = id
		event.ItemKind = agent.ItemToolCall
		event.Output = output
		event.IsError = isError
	})
}

func (parser *appServerEventParser) nativeEvent(now int64, method string, raw json.RawMessage) []agent.Event {
	return []agent.Event{parser.event(now, method, agent.EventProviderNative, raw, func(event *agent.Event) {
		event.Data = cloneRaw(raw)
	})}
}

func (parser *appServerEventParser) event(
	now int64,
	method string,
	eventType agent.EventType,
	raw json.RawMessage,
	configure func(*agent.Event),
) agent.Event {
	ids := nativeIDs(raw)
	event := agent.Event{
		T:              now,
		Type:           eventType,
		Provider:       parser.req.Provider,
		ConversationID: parser.req.ConversationID,
		Raw:            cloneRaw(raw),
		Native: &agent.NativeEnvelope{
			SchemaVersion: agent.NativeEnvelopeSchemaVersion,
			Method:        method,
			ThreadID:      ids.ThreadID,
			TurnID:        ids.TurnID,
			ItemID:        ids.ItemID,
			RequestID:     ids.RequestID,
			Payload:       cloneRaw(raw),
		},
	}
	if configure != nil {
		configure(&event)
	}
	return event
}

func nativeIDs(raw json.RawMessage) appServerNativeIDs {
	var value struct {
		ThreadID string          `json:"threadId"`
		TurnID   string          `json:"turnId"`
		ItemID   string          `json:"itemId"`
		Request  json.RawMessage `json:"requestId"`
		Thread   struct {
			ID string `json:"id"`
		} `json:"thread"`
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
		Item struct {
			ID string `json:"id"`
		} `json:"item"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return appServerNativeIDs{}
	}
	ids := appServerNativeIDs{
		ThreadID: value.ThreadID,
		TurnID:   value.TurnID,
		ItemID:   value.ItemID,
	}
	if ids.ThreadID == "" {
		ids.ThreadID = value.Thread.ID
	}
	if ids.TurnID == "" {
		ids.TurnID = value.Turn.ID
	}
	if ids.ItemID == "" {
		ids.ItemID = value.Item.ID
	}
	ids.RequestID, _ = jsonRPCIDKey(value.Request)
	return ids
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}

func (usage appServerTokenUsage) normalized(model string) json.RawMessage {
	return agent.NormalizeInclusiveInput(agent.Usage{
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CachedInputTokens,
		CacheWriteTokens: usage.CacheWriteInputTokens,
		ReasoningTokens:  usage.ReasoningOutputTokens,
		Model:            model,
	}).Raw()
}

func appServerMCPToolName(item appServerItem) string {
	return joinedToolName(item.Server, item.Tool)
}

func joinedToolName(namespace, tool string) string {
	if namespace == "" {
		return tool
	}
	if tool == "" {
		return namespace
	}
	return namespace + "/" + tool
}

func appServerItemFailed(item appServerItem) bool {
	if item.ExitCode != nil && *item.ExitCode != 0 {
		return true
	}
	status := strings.ToLower(item.Status)
	return status == "failed" || status == "declined" || status == "denied"
}

func appServerToolOutput(item appServerItem) string {
	if len(item.Error) > 0 && string(item.Error) != "null" {
		return compactJSON(item.Error)
	}
	if len(item.Result) > 0 && string(item.Result) != "null" {
		return compactJSON(item.Result)
	}
	return item.Status
}

func compactJSON(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return string(raw)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func mustJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error":%q}`, err.Error()))
	}
	return data
}
