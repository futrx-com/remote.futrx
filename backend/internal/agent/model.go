package agent

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrRunFailed = errors.New("agent run failed")
var ErrSessionNotFound = errors.New("agent session not found")

type ProviderID string

const (
	ProviderClaude      ProviderID = "claude"
	ProviderCodex       ProviderID = "codex"
	ProviderMiniMax     ProviderID = "minimax"
	ProviderKimi        ProviderID = "kimi"
	ProviderAntigravity ProviderID = "antigravity"
)

type EventType string

const (
	EventRunStarted         EventType = "run.started"
	EventRunCompleted       EventType = "run.completed"
	EventRunInterrupted     EventType = "run.interrupted"
	EventRunFailed          EventType = "run.failed"
	EventSessionUpdated     EventType = "session.updated"
	EventSystem             EventType = "system"
	EventAssistantTextDelta EventType = "assistant.delta"
	EventReasoningDelta     EventType = "reasoning.delta"
	EventToolStarted        EventType = "tool.started"
	EventToolCompleted      EventType = "tool.completed"
	EventUsageUpdated       EventType = "usage.updated"
	EventError              EventType = "error"
	EventProviderNative     EventType = "provider.native"
	EventInteractionRequest EventType = "interaction.request"
	EventInteractionDone    EventType = "interaction.resolved"
	EventTurnStatus         EventType = "turn.status"
	EventCollaboration      EventType = "collaboration"
)

type ItemKind string

const (
	ItemMessage   ItemKind = "message"
	ItemReasoning ItemKind = "reasoning"
	ItemToolCall  ItemKind = "tool_call"
	ItemSystem    ItemKind = "system"
)

type ReasoningEffort string
type ServiceTier string
type RunMode string

const (
	RunModeDefault RunMode = "default"
	RunModePlan    RunMode = "plan"
)

// RunPreferences contains provider-neutral launch preferences. Each provider
// adapter decides which preferences to forward and how to translate them.
type RunPreferences struct {
	ReasoningEffort ReasoningEffort
	ServiceTier     ServiceTier
	ApprovalPolicy  string
	SandboxPolicy   string
}

// InteractionResponse is an explicit client answer to a server-initiated
// provider request. ID is a stable JSON representation of the upstream
// JSON-RPC request ID, so string and numeric IDs remain distinct and the
// provider can answer the exact pending request.
type InteractionResponse struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

// NativeEnvelopeSchemaVersion identifies the provider-native correlation
// contract persisted with normalized agent events.
const NativeEnvelopeSchemaVersion = 1

// NativeEnvelope retains provider-owned protocol data without forcing it
// through the provider-neutral event vocabulary. Payload contains the native
// notification params, not binary attachments or client response secrets.
type NativeEnvelope struct {
	SchemaVersion int             `json:"schemaVersion"`
	Method        string          `json:"method"`
	ThreadID      string          `json:"threadId,omitempty"`
	TurnID        string          `json:"turnId,omitempty"`
	ItemID        string          `json:"itemId,omitempty"`
	RequestID     string          `json:"requestId,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

// RunRequest is provider-neutral. Provider adapters translate it into the
// concrete CLI flags and runtime setup required by Claude Code, Codex, etc.
type RunRequest struct {
	Provider       ProviderID
	ConversationID string
	Prompt         string
	Cwd            string
	Model          string
	Mode           RunMode
	ResumeID       string
	ProjectID      string
	Fork           bool
	Preferences    RunPreferences
	// EnableBrowser wires the Agent Browser MCP tools into the agent launch.
	// Set when the `browser` skill is selected for the prompt.
	EnableBrowser bool
	// EnableScheduleTools ensures the provider-neutral remote-schedule CLI and
	// its skill are present for this run.
	EnableScheduleTools bool
	// RuntimeEnv carries short-lived, backend-issued capabilities into a run.
	// Provider adapters must not persist these values in project configuration.
	RuntimeEnv map[string]string
	// InteractionResponses carries UI answers back to server-initiated
	// requests while the provider turn remains active.
	InteractionResponses <-chan InteractionResponse
}

// Event is the normalized backend event shape emitted by headless agent
// providers. Transport-specific chat events are derived from this at the edge.
type Event struct {
	T              int64           `json:"t"`
	Type           EventType       `json:"type"`
	Provider       ProviderID      `json:"provider,omitempty"`
	ConversationID string          `json:"conversationId,omitempty"`
	RunID          string          `json:"runId,omitempty"`
	SessionID      string          `json:"sessionId,omitempty"`
	MessageID      string          `json:"messageId,omitempty"`
	ItemID         string          `json:"itemId,omitempty"`
	ItemKind       ItemKind        `json:"itemKind,omitempty"`
	Role           string          `json:"role,omitempty"`
	Text           string          `json:"text,omitempty"`
	Message        string          `json:"message,omitempty"`
	Subtype        string          `json:"subtype,omitempty"`
	Model          string          `json:"model,omitempty"`
	ToolName       string          `json:"toolName,omitempty"`
	Input          json.RawMessage `json:"input,omitempty"`
	Output         string          `json:"output,omitempty"`
	IsError        bool            `json:"isError,omitempty"`
	Data           json.RawMessage `json:"data,omitempty"`
	Usage          json.RawMessage `json:"usage,omitempty"`
	Raw            json.RawMessage `json:"raw,omitempty"`
	Native         *NativeEnvelope `json:"native,omitempty"`
	InteractionID  string          `json:"interactionId,omitempty"`
	Status         string          `json:"status,omitempty"`
}

type CapabilityProvider interface {
	ID() ProviderID
	Capabilities(ctx context.Context, req CapabilityRequest) (Capabilities, error)
}

type Provider interface {
	CapabilityProvider
	Run(ctx context.Context, req RunRequest, emit func(Event)) error
}
