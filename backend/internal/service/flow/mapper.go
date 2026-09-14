package flow

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type FlowMapper struct{}

func NewFlowMapper() *FlowMapper {
	return &FlowMapper{}
}

func (m *FlowMapper) BuildState(chatID string, events []servicechat.Event) FlowMapState {
	state := FlowMapState{
		ChatID:      chatID,
		Checkpoints: []Checkpoint{},
		Nodes:       []FlowNode{},
		Edges:       []FlowEdge{},
		ProcessTree: []ProcessTreeNode{},
		LastUpdated: time.Now().UnixMilli(),
	}

	if len(events) == 0 {
		return state
	}

	nodeMap := make(map[string]*FlowNode)
	var orderNodes []FlowNode
	var currentCheckpoint *Checkpoint
	var prevNodeID string
	orderCount := 0

	// Helper to add binary path string (e.g. 0, 00, 01, 010)
	formatBinaryPath := func(idx int) string {
		if idx == 0 {
			return "0"
		}
		res := ""
		val := idx
		for val > 0 {
			res = fmt.Sprintf("%d", val%2) + res
			val /= 2
		}
		return "0" + res
	}

	for _, ev := range events {
		seq := ev.Seq
		ts := ev.T
		if ts == 0 {
			ts = time.Now().UnixMilli()
		}

		switch ev.Type {
		case "user", "prompt":
			// User turn start -> Create Conductor Checkpoint
			cpID := fmt.Sprintf("cp_%d", seq)
			cpName := servicechat.TitleFromPrompt(ev.Text)
			if cpName == "" {
				cpName = fmt.Sprintf("Phase %d", len(state.Checkpoints)+1)
			}
			cp := Checkpoint{
				ID:        cpID,
				Name:      cpName,
				Report:    ev.Text,
				Status:    "active",
				NodeIDs:   []string{},
				Timestamp: ts,
			}
			if currentCheckpoint != nil && currentCheckpoint.Status == "active" {
				currentCheckpoint.Status = "completed"
			}
			state.Checkpoints = append(state.Checkpoints, cp)
			currentCheckpoint = &state.Checkpoints[len(state.Checkpoints)-1]
			state.ActiveCheckpoint = cpID

			// Build Maestro Process Tree branch
			treeNode := ProcessTreeNode{
				ID:         fmt.Sprintf("tree_%d", seq),
				BinaryPath: formatBinaryPath(len(state.ProcessTree)),
				Title:      cpName,
				Status:     "processing",
				Priority:   "high",
			}
			state.ProcessTree = append(state.ProcessTree, treeNode)

		case "tool_call", "tool":
			// Tool execution -> Order Node
			orderCount++
			nodeID := fmt.Sprintf("node_%s_%d", ev.ID, seq)
			if ev.ID == "" {
				nodeID = fmt.Sprintf("node_%d", seq)
			}

			verb, icon, title, payload := parseToolDetails(ev)
			bPath := formatBinaryPath(orderCount)

			status := StatusProcessing
			if ev.Output != "" || ev.Subtype == "result" {
				if ev.IsError {
					status = StatusFailed
				} else {
					status = StatusCompleted
				}
			}

			node := FlowNode{
				ID:          nodeID,
				Seq:         seq,
				TurnID:      ev.TurnID,
				Type:        NodeTypeOrder,
				Title:       title,
				Description: ev.ToolName,
				Verb:        verb,
				Status:      status,
				Icon:        icon,
				BinaryPath:  bPath,
				Timestamp:   ts,
				Payload:     payload,
				AuditTrail: []AuditEntry{
					{
						Verb:      verb,
						Actor:     "agent",
						Status:    string(status),
						Timestamp: time.UnixMilli(ts),
					},
				},
			}

			// If existing node (update output)
			if existing, ok := nodeMap[nodeID]; ok {
				existing.Status = status
				if ev.Output != "" {
					existing.Payload["output"] = ev.Output
				}
				if ev.IsError {
					existing.Status = StatusFailed
				}
			} else {
				nodeMap[nodeID] = &node
				orderNodes = append(orderNodes, node)

				if currentCheckpoint != nil {
					currentCheckpoint.NodeIDs = append(currentCheckpoint.NodeIDs, nodeID)
				}

				// Causal Edge wiring
				if prevNodeID != "" {
					edge := FlowEdge{
						ID:         fmt.Sprintf("edge_%s_%s", prevNodeID, nodeID),
						FromNodeID: prevNodeID,
						ToNodeID:   nodeID,
						Type:       "causal",
					}
					state.Edges = append(state.Edges, edge)
				}
				prevNodeID = nodeID
				state.ActiveTargetNodeID = nodeID
			}

		case "ask_user_question", "confirm_gate":
			// Human-in-the-loop Gate
			orderCount++
			nodeID := fmt.Sprintf("gate_%d", seq)
			bPath := formatBinaryPath(orderCount)
			node := FlowNode{
				ID:          nodeID,
				Seq:         seq,
				TurnID:      ev.TurnID,
				Type:        NodeTypeGate,
				Title:       "Human Confirmation Gate",
				Description: ev.Text,
				Verb:        "confirm",
				Status:      StatusGated,
				Icon:        "🛑",
				BinaryPath:  bPath,
				Timestamp:   ts,
				Payload: map[string]interface{}{
					"question": ev.Text,
				},
			}
			nodeMap[nodeID] = &node
			orderNodes = append(orderNodes, node)
			if prevNodeID != "" {
				state.Edges = append(state.Edges, FlowEdge{
					ID:         fmt.Sprintf("edge_%s_%s", prevNodeID, nodeID),
					FromNodeID: prevNodeID,
					ToNodeID:   nodeID,
					Type:       "sequence",
				})
			}
			prevNodeID = nodeID
			state.ActiveTargetNodeID = nodeID
		}
	}

	state.Nodes = orderNodes
	return state
}

func parseToolDetails(ev servicechat.Event) (verb, icon, title string, payload map[string]interface{}) {
	payload = make(map[string]interface{})
	toolName := strings.ToLower(ev.ToolName)
	if toolName == "" {
		toolName = strings.ToLower(ev.Name)
	}

	if ev.Input != nil {
		var rawInput map[string]interface{}
		if err := json.Unmarshal(ev.Input, &rawInput); err == nil {
			payload = rawInput
		}
	}

	switch {
	case strings.Contains(toolName, "navigate"):
		verb = "navigate"
		icon = "🌐"
		urlStr, _ := payload["url"].(string)
		title = fmt.Sprintf("Navigate to %s", truncateString(urlStr, 30))

	case strings.Contains(toolName, "click"):
		verb = "click"
		icon = "🖱️"
		ref, _ := payload["ref"].(string)
		title = fmt.Sprintf("Click target %s", ref)

	case strings.Contains(toolName, "snapshot"):
		verb = "snapshot"
		icon = "🔍"
		title = "Capture DOM Snapshot"

	case strings.Contains(toolName, "screenshot"):
		verb = "screenshot"
		icon = "📸"
		title = "Take Visual Screenshot"

	case strings.Contains(toolName, "human-input") || strings.Contains(toolName, "type"):
		verb = "type"
		icon = "⌨️"
		text, _ := payload["text"].(string)
		title = fmt.Sprintf("Computer Input: %s", truncateString(text, 25))

	case strings.Contains(toolName, "write"):
		verb = "write"
		icon = "📝"
		path, _ := payload["path"].(string)
		title = fmt.Sprintf("Write %s", truncateString(path, 25))

	case strings.Contains(toolName, "edit"):
		verb = "edit"
		icon = "✏️"
		path, _ := payload["path"].(string)
		title = fmt.Sprintf("Edit %s", truncateString(path, 25))

	case strings.Contains(toolName, "bash"):
		verb = "execute"
		icon = "💻"
		cmd, _ := payload["command"].(string)
		title = fmt.Sprintf("Bash: %s", truncateString(cmd, 25))

	default:
		verb = "execute"
		icon = "⚡"
		title = fmt.Sprintf("Execute %s", toolName)
	}

	return verb, icon, title, payload
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
