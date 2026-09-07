package kimi

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

// agentActivity owns the delegated-agent graph, correlated tools and main-owned
// task controls. Native nested task termination also updates the graph.
type agentActivity struct {
	children map[string]*childAgent
	tools    map[string]*childTool
	tasks    map[string]bool
}

func newAgentActivity() agentActivity {
	return agentActivity{children: map[string]*childAgent{}, tools: map[string]*childTool{}, tasks: map[string]bool{}}
}

type childTool struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Status      string          `json:"status"`
	Input       json.RawMessage `json:"input,omitempty"`
	Output      string          `json:"output,omitempty"`
	IsError     bool            `json:"isError,omitempty"`
	StartedAt   int64           `json:"startedAt,omitempty"`
	CompletedAt int64           `json:"completedAt,omitempty"`
}
type childAgent struct {
	sideConversation                                                          bool
	lastEmit                                                                  time.Time
	id, name, parent, parentTool, taskID, status, text, thinking, description string
	background                                                                bool
	tools                                                                     []*childTool
}

func (a *agentActivity) child(id string) *childAgent {
	c := a.children[id]
	if c == nil {
		c = &childAgent{id: id, name: id, parent: "main", status: "running"}
		a.children[id] = c
	}
	return c
}
func (a *agentActivity) collaboration(session string, c *childAgent, native *agent.NativeEnvelope) *agent.Event {
	if native != nil && (native.Method == "kimi/assistant.delta" || native.Method == "kimi/thinking.delta") && time.Since(c.lastEmit) < configconstants.KimiChildDeltaInterval {
		return nil
	}
	c.lastEmit = time.Now()
	failed := 0
	for _, t := range c.tools {
		if t.IsError {
			failed++
		}
	}
	data, _ := json.Marshal(map[string]any{
		"type": "subagentThread", "senderThreadId": session + ":" + c.parent, "receiverThreadIds": []string{session + ":" + c.id},
		"agentsStates": map[string]any{session + ":" + c.id: map[string]string{"status": c.status, "message": c.text}},
		"prompt":       c.description, "agentNickname": c.name, "agentRole": c.name, "parentAgentId": c.parent, "parentToolCallId": c.parentTool, "taskId": c.taskID,
		"stopInteractionId": a.taskControlID(session, c), "runInBackground": c.background, "reasoning": c.thinking, "toolCount": len(c.tools), "failedToolCount": failed, "tools": c.tools,
	})
	return &agent.Event{Type: agent.EventCollaboration, ItemID: "subagent:" + session + ":" + c.id, ToolName: c.name, Status: c.status, Data: data, Native: native}
}
func (a *agentActivity) taskControlID(session string, c *childAgent) string {
	if c.taskID != "" && a.tasks[c.taskID] {
		return "task:" + session + ":" + c.taskID
	}
	return ""
}

func (a *agentActivity) task(kind, who string, raw json.RawMessage) *childAgent {
	var task struct {
		Info struct {
			ID      string `json:"taskId"`
			AgentID string `json:"agentId"`
			Status  string `json:"status"`
		} `json:"info"`
	}
	if json.Unmarshal(raw, &task) == nil && task.Info.ID != "" {
		// The public task API controls main-owned tasks. Still reconcile
		// nested task termination: cancellation has no subagent.failed event.
		if who == "main" {
			if kind == "task.started" {
				a.tasks[task.Info.ID] = true
			} else {
				delete(a.tasks, task.Info.ID)
			}
		}
		if task.Info.AgentID != "" && task.Info.AgentID != "main" {
			c := a.child(task.Info.AgentID)
			c.taskID = task.Info.ID
			if kind == "task.terminated" {
				switch task.Info.Status {
				case "killed":
					c.status = "cancelled"
				case "failed", "timed_out", "lost":
					c.status = "failed"
				case "completed":
					c.status = "completed"
				}
			}
			return c
		}
	}
	return nil
}
func (a *agentActivity) startSideConversation(id, question string) *childAgent {
	c := a.child(id)
	c.name, c.description, c.sideConversation = "Side question", question, true
	return c
}
func (a *agentActivity) endSideConversation(who string, p nativePayload) (*childAgent, string, bool) {
	failure := ""
	interrupted := false

	c := a.child(who)
	c.status = p.Reason
	if p.Reason == "failed" || p.Reason == "blocked" {
		failure = nativeText(p.Error)
		if failure == "" {
			failure = "Kimi side question " + p.Reason
		}
		c.status, c.text = "failed", failure
	}
	if p.Reason == "cancelled" {
		interrupted = true
	}
	return c, failure, interrupted
}
func (a *agentActivity) delta(kind, who, text string) *childAgent {
	c := a.child(who)
	if kind == "assistant.delta" {
		c.text += text
	} else {
		c.thinking += text
	}
	return c
}
func (a *agentActivity) startTool(session, who string, p nativePayload) *childTool {
	id := fmt.Sprintf("%s:%s:%d:%s", session, who, p.TurnID, p.ToolCallID)
	t := &childTool{ID: id, Name: p.Name, Status: "running", Input: p.Args, StartedAt: time.Now().UnixMilli()}
	a.tools[id] = t
	if who != "main" {
		c := a.child(who)
		c.tools = append(c.tools, t)
	}
	return t
}
func (a *agentActivity) finishTool(session, who string, p nativePayload) (string, *childTool) {
	id := fmt.Sprintf("%s:%s:%d:%s", session, who, p.TurnID, p.ToolCallID)
	t := a.tools[id]
	if t != nil {
		t.Status = "completed"
		t.Output = nativeText(p.Output)
		t.IsError = p.IsError
		t.CompletedAt = time.Now().UnixMilli()
		if t.IsError {
			t.Status = "failed"
		}
	}
	return id, t
}
func (a *agentActivity) spawn(p nativePayload) *childAgent {
	c := a.child(p.SubagentID)
	c.name = p.SubagentName
	c.description = p.Description
	c.parent = p.ParentAgentID
	if c.parent == "" {
		c.parent = p.CallerAgentID
	}
	if c.parent == "" {
		c.parent = "main"
	}
	c.parentTool = p.ParentToolCallID
	c.taskID = p.TaskID
	c.background = p.RunInBackground
	c.status = "running"
	return c
}
func (a *agentActivity) lifecycle(kind string, p nativePayload) *childAgent {
	c := a.child(p.SubagentID)
	switch kind {
	case "subagent.started":
		c.status = "running"
	case "subagent.suspended":
		c.status = "waiting"
	case "subagent.completed":
		c.status = "completed"
		if p.ResultSummary != "" {
			c.text = p.ResultSummary
		}
	case "subagent.failed":
		c.status = "failed"
		c.text = nativeText(p.Error)
	}
	// Child completion usage is cumulative; runUsage already includes it.
	return c
}
func (a *agentActivity) running() bool {
	for _, c := range a.children {
		if c.status == "running" {
			return true
		}
	}
	return false
}
func (a *agentActivity) controlsTask(id string) bool { return a.tasks[id] }
func (a *agentActivity) detachTask(id string, publish func(*childAgent)) {
	for _, c := range a.children {
		if c.taskID == id {
			c.background = true
			publish(c)
		}
	}
}

func (a *agentActivity) isSideConversation(id string) bool { return a.child(id).sideConversation }
