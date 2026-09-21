package devinharness

import (
	"encoding/json"
)

// acpEnvelope is the JSON-RPC 2.0 envelope shared by requests, responses, and
// notifications. A message with Method and ID is a request; with Method and no
// ID is a notification; with ID and no Method is a response.
type acpEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *acpError       `json:"error,omitempty"`
}

type acpError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// ── initialize ──

type acpClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type acpClientCapabilities struct {
	Elicitation *acpElicitationCapability `json:"elicitation,omitempty"`
	FS          *acpFSCapability          `json:"fs,omitempty"`
}

type acpElicitationCapability struct {
	Form struct{} `json:"form"`
}

type acpFSCapability struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

type acpInitializeParams struct {
	ProtocolVersion    int                   `json:"protocolVersion"`
	ClientCapabilities acpClientCapabilities `json:"clientCapabilities"`
	ClientInfo         acpClientInfo         `json:"clientInfo"`
}

type acpAgentCapabilities struct {
	LoadSession        bool                  `json:"loadSession"`
	PromptCapabilities acpPromptCapabilities `json:"promptCapabilities"`
	MCPCapabilities    acpMCPCapabilities    `json:"mcpCapabilities"`
}

type acpPromptCapabilities struct {
	Image           bool `json:"image"`
	Audio           bool `json:"audio"`
	EmbeddedContext bool `json:"embeddedContext"`
}

type acpMCPCapabilities struct {
	HTTP bool `json:"http"`
	SSE  bool `json:"sse"`
}

type acpAuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type acpAgentInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type acpInitializeResult struct {
	ProtocolVersion  int                 `json:"protocolVersion"`
	AgentCapabilities acpAgentCapabilities `json:"agentCapabilities"`
	AuthMethods      []acpAuthMethod     `json:"authMethods"`
	AgentInfo        acpAgentInfo        `json:"agentInfo"`
}

// ── session/new and session/resume ──

type acpSessionNewParams struct {
	Cwd        string             `json:"cwd"`
	McpServers []json.RawMessage  `json:"mcpServers"`
}

type acpSessionResumeParams struct {
	SessionID  string            `json:"sessionId"`
	Cwd        string            `json:"cwd"`
	McpServers []json.RawMessage `json:"mcpServers"`
}

type acpSessionResult struct {
	SessionID       string          `json:"sessionId"`
	ConfigOptions   json.RawMessage `json:"configOptions,omitempty"`
}

// ── session/prompt ──

type acpContentBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text,omitempty"`
	Image        json.RawMessage `json:"image,omitempty"`
	ResourceLink json.RawMessage `json:"resourceLink,omitempty"`
}

type acpSessionPromptParams struct {
	SessionID string             `json:"sessionId"`
	Prompt    []acpContentBlock  `json:"prompt"`
}

type acpStopReason string

const (
	StopReasonEndTurn          acpStopReason = "end_turn"
	StopReasonMaxTokens        acpStopReason = "max_tokens"
	StopReasonMaxTurnRequests  acpStopReason = "max_turn_requests"
	StopReasonRefusal          acpStopReason = "refusal"
	StopReasonCancelled        acpStopReason = "cancelled"
)

type acpSessionPromptResult struct {
	StopReason acpStopReason   `json:"stopReason"`
	Usage      json.RawMessage `json:"usage,omitempty"`
}

// ── session/cancel (notification) ──

type acpSessionCancelParams struct {
	SessionID string `json:"sessionId"`
}

// ── session/update notification ──

type acpSessionNotificationParams struct {
	SessionID string          `json:"sessionId"`
	Update    json.RawMessage `json:"update"`
}

// SessionUpdate is the union discriminated by the sessionUpdate string field.
// We decode the discriminator first, then parse the variant.
type acpSessionUpdate struct {
	SessionUpdate string          `json:"sessionUpdate"`
	Content       json.RawMessage `json:"content,omitempty"`
	MessageID     string          `json:"messageId,omitempty"`
	ToolCallID    string          `json:"toolCallId,omitempty"`
	Title         string          `json:"title,omitempty"`
	Kind          string          `json:"kind,omitempty"`
	RawInput      json.RawMessage `json:"rawInput,omitempty"`
	RawOutput     json.RawMessage `json:"rawOutput,omitempty"`
	Status        string          `json:"status,omitempty"`
	Entries       json.RawMessage `json:"entries,omitempty"`
	AvailableCommands json.RawMessage `json:"availableCommands,omitempty"`
	CurrentModeID string          `json:"currentModeId,omitempty"`
	ConfigOptions json.RawMessage `json:"configOptions,omitempty"`
}

// ── session/request_permission (server-to-client request) ──

type acpPermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name,omitempty"`
	Kind     string `json:"kind,omitempty"`
}

type acpRequestPermissionParams struct {
	SessionID string                `json:"sessionId"`
	ToolCall  json.RawMessage       `json:"toolCall"`
	Options   []acpPermissionOption `json:"options"`
}

// acpPermissionResult is the response sent back to the ACP server. The
// outcome field is the struct field of RequestPermissionResponse, and its
// value is the internally-tagged enum RequestPermissionOutcome.
//
// ACP v1 schema:
//   struct RequestPermissionResponse { outcome: RequestPermissionOutcome }
//   #[serde(tag = "outcome", rename_all = "snake_case")]
//   enum RequestPermissionOutcome { Cancelled, Selected(SelectedPermissionOutcome) }
//
// So the wire format is:
//   {"outcome": {"outcome": "selected", "optionId": "allow_once"}}
//   {"outcome": {"outcome": "cancelled"}}
type acpPermissionResult struct {
	Outcome acpPermissionOutcome `json:"outcome"`
}

type acpPermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

// ── session/elicitation/create (server-to-client request) ──

type acpCreateElicitationParams struct {
	ElicitationID   string          `json:"elicitationId"`
	RequestedSchema json.RawMessage `json:"requestedSchema"`
	ToolCallID      string          `json:"toolCallId,omitempty"`
}

type acpElicitationResult struct {
	Action  string          `json:"action"`
	Content json.RawMessage `json:"content,omitempty"`
}
