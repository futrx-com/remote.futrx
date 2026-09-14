package flow

import (
	"encoding/json"
	"time"
)

// FlowNodeType identifies the type of visual node in the Flow Map.
type FlowNodeType string

const (
	NodeTypeCheckpoint FlowNodeType = "checkpoint"
	NodeTypeOrder      FlowNodeType = "order"
	NodeTypeTreeNode   FlowNodeType = "tree_node"
	NodeTypeGate       FlowNodeType = "gate"
)

// FlowNodeStatus represents the execution lifecycle of a work unit/order.
type FlowNodeStatus string

const (
	StatusCreated    FlowNodeStatus = "created"
	StatusProcessing FlowNodeStatus = "processing"
	StatusCompleted  FlowNodeStatus = "completed"
	StatusFailed     FlowNodeStatus = "failed"
	StatusGated      FlowNodeStatus = "gated"
)

// AuditEntry tracks historical verb execution and timestamp for an Order.
type AuditEntry struct {
	Verb      string    `json:"verb"`
	Actor     string    `json:"actor"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Note      string    `json:"note,omitempty"`
}

// FlowNode is an atomic unit of visual work (Order / Checkpoint / Process Node).
type FlowNode struct {
	ID          string                 `json:"id"`
	Seq         int64                  `json:"seq"`
	TurnID      string                 `json:"turnId,omitempty"`
	Type        FlowNodeType           `json:"type"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Verb        string                 `json:"verb"`
	Status      FlowNodeStatus         `json:"status"`
	Icon        string                 `json:"icon"`
	BinaryPath  string                 `json:"binaryPath,omitempty"` // Maestro tree bitstring address (e.g. "0101")
	Timestamp   int64                  `json:"timestamp"`
	DurationMs  int64                  `json:"durationMs,omitempty"`
	ParentID    string                 `json:"parentId,omitempty"`
	Payload     map[string]interface{} `json:"payload,omitempty"` // Coordinates, URL, selectors, screenshot info
	AuditTrail  []AuditEntry           `json:"auditTrail,omitempty"`
}

// FlowEdge describes causal or structural connections between visual nodes.
type FlowEdge struct {
	ID         string `json:"id"`
	FromNodeID string `json:"fromNodeId"`
	ToNodeID   string `json:"toNodeId"`
	Label      string `json:"label,omitempty"`
	Type       string `json:"type"` // "causal", "tree", "sequence"
}

// Checkpoint represents a Conductor stage with an unloaded/loaded report envelope.
type Checkpoint struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Report    string   `json:"report,omitempty"`
	Status    string   `json:"status"`
	NodeIDs   []string `json:"nodeIds"`
	Timestamp int64    `json:"timestamp"`
}

// ProcessTreeNode is a Maestro process tree node mapping system/feature decomposition.
type ProcessTreeNode struct {
	ID         string            `json:"id"`
	BinaryPath string            `json:"binaryPath"` // Bitstring address
	Title      string            `json:"title"`
	Status     string            `json:"status"`
	Priority   string            `json:"priority,omitempty"`
	Children   []ProcessTreeNode `json:"children,omitempty"`
}

// FlowMapState is the complete graph snapshot for a chat session.
type FlowMapState struct {
	ChatID            string            `json:"chatId"`
	ActiveCheckpoint  string            `json:"activeCheckpoint,omitempty"`
	Checkpoints       []Checkpoint      `json:"checkpoints"`
	Nodes             []FlowNode        `json:"nodes"`
	Edges             []FlowEdge        `json:"edges"`
	ProcessTree       []ProcessTreeNode `json:"processTree"`
	ActiveTargetNodeID string           `json:"activeTargetNodeId,omitempty"`
	LastUpdated       int64             `json:"lastUpdated"`
}
