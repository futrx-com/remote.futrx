package codexharness

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type appServerSubagentTracker struct {
	parser       *appServerEventParser
	rootThreadID string
	states       map[string]*appServerSubagentState
}

type appServerSubagentState struct {
	threadID      string
	name          string
	role          string
	status        string
	message       string
	toolOrder     []string
	tools         map[string]*appServerSubagentTool
	failedToolIDs map[string]struct{}
}

type appServerSubagentTool struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Status      string          `json:"status"`
	Input       json.RawMessage `json:"input,omitempty"`
	Output      string          `json:"output,omitempty"`
	IsError     bool            `json:"isError,omitempty"`
	StartedAt   int64           `json:"startedAt,omitempty"`
	CompletedAt int64           `json:"completedAt,omitempty"`
	DurationMs  *int64          `json:"durationMs,omitempty"`
}

func newAppServerSubagentTracker(parser *appServerEventParser) *appServerSubagentTracker {
	return &appServerSubagentTracker{
		parser: parser,
		states: make(map[string]*appServerSubagentState),
	}
}

func (tracker *appServerSubagentTracker) ParseNotification(
	rootThreadID string,
	method string,
	raw json.RawMessage,
) []agent.Event {
	now := time.Now().UnixMilli()
	if rootThreadID != "" {
		tracker.rootThreadID = rootThreadID
	}
	ids := nativeIDs(raw)
	if ids.ThreadID == "" {
		return tracker.parser.nativeEvent(now, method, raw)
	}
	state := tracker.state(ids.ThreadID)
	changed := false

	switch method {
	case "thread/started":
		var params appServerThreadStartedParams
		if json.Unmarshal(raw, &params) == nil {
			state.name = stringValue(params.Thread.AgentNickname)
			state.role = stringValue(params.Thread.AgentRole)
			changed = true
		}

	case "thread/status/changed":
		var params appServerThreadStatusParams
		if json.Unmarshal(raw, &params) == nil {
			nextStatus := subagentThreadStatus(params.Status.Type)
			// An idle thread notification commonly follows turn/completed. Keep
			// the authoritative terminal turn status, while still allowing a
			// later active/turn-started notification to represent a follow-up.
			if nextStatus != "idle" || !isTerminalSubagentStatus(state.status) {
				state.status = nextStatus
			}
			changed = true
		}

	case "turn/started":
		state.status = "inProgress"
		changed = true

	case "turn/completed":
		var params appServerTurnCompletedParams
		if json.Unmarshal(raw, &params) == nil {
			state.status = strings.TrimSpace(params.Turn.Status)
			if state.status == "" {
				state.status = "completed"
			}
			state.captureMessages(params.Turn.Items)
			changed = true
		}

	case "item/started", "item/completed":
		var params appServerItemParams
		if json.Unmarshal(raw, &params) != nil {
			break
		}
		if params.Item.Type == "agentMessage" || params.Item.Type == "plan" {
			if method == "item/completed" && strings.TrimSpace(params.Item.Text) != "" {
				state.message = params.Item.Text
				changed = true
			}
			break
		}
		if subagentWorkItem(params.Item.Type) {
			state.updateTool(params.Item, method == "item/completed", now)
			if !isTerminalSubagentStatus(state.status) {
				state.status = "inProgress"
			}
			changed = true
		}
	}

	if !changed {
		return tracker.parser.nativeEvent(now, method, raw)
	}
	return []agent.Event{tracker.event(now, rootThreadID, method, raw, state)}
}

func (tracker *appServerSubagentTracker) Finalize(rootStatus string) []agent.Event {
	threadIDs := make([]string, 0, len(tracker.states))
	for threadID := range tracker.states {
		threadIDs = append(threadIDs, threadID)
	}
	sort.Strings(threadIDs)

	var events []agent.Event
	for _, threadID := range threadIDs {
		state := tracker.states[threadID]
		if isTerminalSubagentStatus(state.status) {
			continue
		}
		nextStatus := rootStatus
		if rootStatus == "completed" {
			if state.status == "idle" {
				nextStatus = "completed"
			} else {
				nextStatus = "turnEnded"
			}
		}
		state.status = nextStatus
		events = append(events, tracker.syntheticEvent(state))
	}
	return events
}

func (tracker *appServerSubagentTracker) state(threadID string) *appServerSubagentState {
	if state := tracker.states[threadID]; state != nil {
		return state
	}
	state := &appServerSubagentState{
		threadID:      threadID,
		status:        "inProgress",
		tools:         make(map[string]*appServerSubagentTool),
		failedToolIDs: make(map[string]struct{}),
	}
	tracker.states[threadID] = state
	return state
}

func (tracker *appServerSubagentTracker) event(
	now int64,
	rootThreadID string,
	method string,
	raw json.RawMessage,
	state *appServerSubagentState,
) agent.Event {
	return tracker.parser.event(now, method, agent.EventCollaboration, raw, func(event *agent.Event) {
		event.ItemID = subagentEventID(state.threadID)
		event.ItemKind = agent.ItemToolCall
		event.ToolName = state.label()
		event.Status = state.status
		event.Data = state.data(rootThreadID)
	})
}

func (tracker *appServerSubagentTracker) syntheticEvent(state *appServerSubagentState) agent.Event {
	return agent.Event{
		T:              time.Now().UnixMilli(),
		Type:           agent.EventCollaboration,
		Provider:       tracker.parser.req.Provider,
		ConversationID: tracker.parser.req.ConversationID,
		ItemID:         subagentEventID(state.threadID),
		ItemKind:       agent.ItemToolCall,
		ToolName:       state.label(),
		Status:         state.status,
		Data:           state.data(tracker.rootThreadID),
	}
}

func (state *appServerSubagentState) data(rootThreadID string) json.RawMessage {
	agentState := map[string]any{"status": state.status}
	if strings.TrimSpace(state.message) != "" {
		agentState["message"] = state.message
	}
	tools := make([]appServerSubagentTool, 0, len(state.toolOrder))
	for _, id := range state.toolOrder {
		if tool := state.tools[id]; tool != nil {
			tools = append(tools, *tool)
		}
	}
	data := map[string]any{
		"type":              "subagentThread",
		"receiverThreadIds": []string{state.threadID},
		"agentsStates":      map[string]any{state.threadID: agentState},
		"toolCount":         len(tools),
		"failedToolCount":   len(state.failedToolIDs),
		"tools":             tools,
	}
	if rootThreadID != "" {
		data["senderThreadId"] = rootThreadID
	}
	if state.name != "" {
		data["agentNickname"] = state.name
	}
	if state.role != "" {
		data["agentRole"] = state.role
	}
	return mustJSON(data)
}

func (state *appServerSubagentState) captureMessages(items []appServerItem) {
	for _, item := range items {
		if (item.Type == "agentMessage" || item.Type == "plan") && strings.TrimSpace(item.Text) != "" {
			state.message = item.Text
		}
	}
}

func (state *appServerSubagentState) updateTool(item appServerItem, completed bool, now int64) {
	tool := state.tools[item.ID]
	if tool == nil {
		tool = &appServerSubagentTool{ID: item.ID, Name: subagentToolName(item)}
		state.tools[item.ID] = tool
		state.toolOrder = append(state.toolOrder, item.ID)
	}
	if input := subagentToolInput(item); len(input) > 0 {
		tool.Input = cloneRaw(input)
	}
	if completed {
		tool.Status = strings.TrimSpace(item.Status)
		if tool.Status == "" {
			tool.Status = "completed"
		}
		tool.Output = subagentToolOutput(item)
		tool.IsError = appServerItemFailed(item)
		tool.CompletedAt = now
		if tool.StartedAt > 0 && now >= tool.StartedAt {
			duration := now - tool.StartedAt
			tool.DurationMs = &duration
		}
		if tool.IsError {
			state.failedToolIDs[item.ID] = struct{}{}
		}
	} else {
		tool.Status = strings.TrimSpace(item.Status)
		if tool.Status == "" {
			tool.Status = "inProgress"
		}
		if tool.StartedAt == 0 {
			tool.StartedAt = now
		}
	}
}

func (state *appServerSubagentState) label() string {
	if strings.TrimSpace(state.name) != "" {
		return state.name
	}
	if strings.TrimSpace(state.role) != "" {
		return state.role
	}
	return "Subagent"
}

func subagentEventID(threadID string) string {
	return "subagent:" + threadID
}

func subagentThreadStatus(status string) string {
	if strings.TrimSpace(status) == "active" {
		return "inProgress"
	}
	if strings.TrimSpace(status) == "" {
		return "inProgress"
	}
	return strings.TrimSpace(status)
}

func isTerminalSubagentStatus(status string) bool {
	switch status {
	case "completed", "failed", "interrupted", "cancelled", "canceled", "turnEnded":
		return true
	default:
		return false
	}
}

func subagentWorkItem(itemType string) bool {
	switch itemType {
	case "commandExecution", "fileChange", "mcpToolCall", "dynamicToolCall", "webSearch":
		return true
	default:
		return false
	}
}

func subagentToolName(item appServerItem) string {
	switch item.Type {
	case "commandExecution":
		return "Bash"
	case "fileChange":
		return "Patch"
	case "mcpToolCall":
		return appServerMCPToolName(item)
	case "dynamicToolCall":
		return joinedToolName(item.Namespace, item.Tool)
	case "webSearch":
		return "WebSearch"
	default:
		return item.Type
	}
}

func subagentToolInput(item appServerItem) json.RawMessage {
	switch item.Type {
	case "commandExecution":
		if strings.TrimSpace(item.Command) == "" {
			return nil
		}
		return mustJSON(map[string]any{"command": item.Command})
	case "fileChange":
		if len(item.Changes) == 0 || string(item.Changes) == "null" {
			return nil
		}
		return mustJSON(map[string]any{"changes": item.Changes})
	case "mcpToolCall", "dynamicToolCall":
		if len(item.Arguments) == 0 || string(item.Arguments) == "null" {
			return nil
		}
		return cloneRaw(item.Arguments)
	case "webSearch":
		if strings.TrimSpace(item.Query) == "" && (len(item.Action) == 0 || string(item.Action) == "null") {
			return nil
		}
		return mustJSON(map[string]any{"query": item.Query, "action": item.Action})
	default:
		return nil
	}
}

func subagentToolOutput(item appServerItem) string {
	switch item.Type {
	case "commandExecution":
		return item.AggregatedOutput
	case "fileChange":
		return item.Status
	case "mcpToolCall", "dynamicToolCall":
		return appServerToolOutput(item)
	case "webSearch":
		if len(item.Error) > 0 && string(item.Error) != "null" {
			return compactJSON(item.Error)
		}
		if len(item.Result) > 0 && string(item.Result) != "null" {
			return compactJSON(item.Result)
		}
		return item.Query
	default:
		return ""
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
