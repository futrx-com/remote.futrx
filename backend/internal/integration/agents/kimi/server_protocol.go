package kimi

import "encoding/json"

type serverEvent struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id"`
	Seq       int64           `json:"seq"`
	Epoch     string          `json:"epoch"`
	Volatile  bool            `json:"volatile"`
	Payload   json.RawMessage `json:"payload"`
}

type nativePayload struct {
	Type             string          `json:"type"`
	AgentID          string          `json:"agentId"`
	TurnID           int64           `json:"turnId"`
	Step             int             `json:"step"`
	StepID           string          `json:"stepId"`
	Delta            string          `json:"delta"`
	ToolCallID       string          `json:"toolCallId"`
	Name             string          `json:"name"`
	Args             json.RawMessage `json:"args"`
	Output           json.RawMessage `json:"output"`
	IsError          bool            `json:"isError"`
	Reason           string          `json:"reason"`
	Error            json.RawMessage `json:"error"`
	SubagentID       string          `json:"subagentId"`
	SubagentName     string          `json:"subagentName"`
	ParentAgentID    string          `json:"parentAgentId"`
	CallerAgentID    string          `json:"callerAgentId"`
	ParentToolCallID string          `json:"parentToolCallId"`
	Description      string          `json:"description"`
	RunInBackground  bool            `json:"runInBackground"`
	TaskID           string          `json:"taskId"`
	ResultSummary    string          `json:"resultSummary"`
	Usage            *struct {
		InputOther         int64 `json:"inputOther"`
		Output             int64 `json:"output"`
		InputCacheRead     int64 `json:"inputCacheRead"`
		InputCacheCreation int64 `json:"inputCacheCreation"`
	} `json:"usage"`
}

func nativeText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var e struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Message != "" {
		if e.Code != "" {
			return e.Code + ": " + e.Message
		}
		return e.Message
	}
	return string(raw)
}
