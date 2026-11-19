// Package conversation defines the history Remote can carry between harnesses.
// It intentionally excludes runtime diagnostics and private native session state.
package conversation

import "encoding/json"

type Record struct {
	Seq        int64           `json:"seq"`
	EndSeq     int64           `json:"endSeq,omitempty"`
	TurnID     string          `json:"turnId,omitempty"`
	Kind       string          `json:"kind"`
	Provider   string          `json:"provider,omitempty"`
	AccountID  string          `json:"accountId,omitempty"`
	MessageID  string          `json:"messageId,omitempty"`
	ToolCallID string          `json:"toolCallId,omitempty"`
	Name       string          `json:"name,omitempty"`
	Text       string          `json:"text,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     string          `json:"output,omitempty"`
	ContentRef string          `json:"contentRef,omitempty"`
	IsError    bool            `json:"isError,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
}

// History is a bounded, ordered part of a branch. Records contain the original
// exposed conversation, not a Remote-created summary or native history import.
type History struct {
	BranchID   string   `json:"branchId"`
	AfterSeq   int64    `json:"afterSeq"`
	ThroughSeq int64    `json:"throughSeq"`
	Records    []Record `json:"records"`
}
