package agent

import "encoding/json"

// UsageSchemaVersion identifies the disjoint token-bucket contract emitted by
// current provider adapters. Older persisted events have no version marker.
const UsageSchemaVersion = 1

// Usage is the provider-neutral token/cost payload carried by
// EventRunCompleted (and EventRunFailed when the CLI still reported a
// partial turn). Provider adapters translate their native shape into this
// one so downstream consumers — the chat UI, the usage ledger, and the
// offline rebuild that re-reads persisted chat events — never have to know
// which CLI produced a run.
//
// The JSON field names intentionally match Claude Code's stream-json
// vocabulary because that shape is already persisted in existing chat event
// logs and already understood by the frontend projector.
type Usage struct {
	SchemaVersion int `json:"schema_version,omitempty"`
	// InputTokens is uncached input. InputTokens, CacheReadTokens, and
	// CacheWriteTokens are mutually exclusive buckets. Provider adapters whose
	// native input count includes cache activity must call
	// NormalizeInclusiveInput before emitting the usage payload.
	InputTokens      int64 `json:"input_tokens,omitempty"`
	OutputTokens     int64 `json:"output_tokens,omitempty"`
	CacheReadTokens  int64 `json:"cache_read_input_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_creation_input_tokens,omitempty"`
	ReasoningTokens  int64 `json:"reasoning_output_tokens,omitempty"`
	// CostUSD is set only when the provider itself reported a price for the
	// turn. A nil value means "unknown" — never "free".
	CostUSD    *float64 `json:"total_cost_usd,omitempty"`
	DurationMs int64    `json:"duration_ms,omitempty"`
	Turns      int64    `json:"num_turns,omitempty"`
	Model      string   `json:"model,omitempty"`
}

// Empty reports whether the payload carries nothing worth persisting.
func (u Usage) Empty() bool {
	u.SchemaVersion = 0
	return u == Usage{}
}

// TotalTokens is the billable token count across every bucket.
func (u Usage) TotalTokens() int64 {
	return u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheWriteTokens
}

// NormalizeInclusiveInput converts a provider payload whose InputTokens field
// includes the cache-read and cache-write subsets into the disjoint buckets
// used by the normalized Usage contract. Codex and OpenAI-compatible payloads
// use inclusive input counts; Claude already reports disjoint buckets and must
// not pass through this conversion.
func NormalizeInclusiveInput(usage Usage) Usage {
	inclusiveInput := max(usage.InputTokens, 0)
	usage.CacheReadTokens = min(max(usage.CacheReadTokens, 0), inclusiveInput)
	remaining := inclusiveInput - usage.CacheReadTokens
	usage.CacheWriteTokens = min(max(usage.CacheWriteTokens, 0), remaining)
	usage.InputTokens = remaining - usage.CacheWriteTokens
	return usage
}

// Raw renders the payload for Event.Usage. Empty payloads render as nil so
// events stay free of `"usage":{}` noise.
func (u Usage) Raw() json.RawMessage {
	if u.Empty() {
		return nil
	}
	u.SchemaVersion = UsageSchemaVersion
	data, err := json.Marshal(u)
	if err != nil {
		return nil
	}
	return data
}

// ParseUsage decodes a persisted or in-flight usage blob. It accepts both the
// normalized shape above and raw provider spellings still present in older
// chat logs. Parsing aliases does not infer whether a provider's native input
// count includes cache buckets; that decision remains in its adapter.
func ParseUsage(raw json.RawMessage) (Usage, bool) {
	if len(raw) == 0 {
		return Usage{}, false
	}
	var wire usageWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Usage{}, false
	}
	usage := Usage{
		SchemaVersion:    wire.SchemaVersion,
		InputTokens:      firstNonZero(wire.InputTokens, wire.PromptTokens),
		OutputTokens:     firstNonZero(wire.OutputTokens, wire.CompletionTokens),
		CacheReadTokens:  firstNonZero(wire.CacheReadTokens, wire.CachedInputTokens),
		CacheWriteTokens: firstNonZero(wire.CacheWriteTokens, wire.CacheWriteInputTokens),
		ReasoningTokens:  wire.ReasoningTokens,
		CostUSD:          wire.CostUSD,
		DurationMs:       wire.DurationMs,
		Turns:            wire.Turns,
		Model:            wire.Model,
	}
	if usage.Empty() {
		return Usage{}, false
	}
	return usage, true
}

// usageWire is the tolerant decode target: the normalized names plus the
// provider-native aliases we have observed on the wire.
type usageWire struct {
	SchemaVersion         int      `json:"schema_version"`
	InputTokens           int64    `json:"input_tokens"`
	OutputTokens          int64    `json:"output_tokens"`
	CacheReadTokens       int64    `json:"cache_read_input_tokens"`
	CacheWriteTokens      int64    `json:"cache_creation_input_tokens"`
	ReasoningTokens       int64    `json:"reasoning_output_tokens"`
	CachedInputTokens     int64    `json:"cached_input_tokens"`
	CacheWriteInputTokens int64    `json:"cache_write_input_tokens"`
	PromptTokens          int64    `json:"prompt_tokens"`
	CompletionTokens      int64    `json:"completion_tokens"`
	CostUSD               *float64 `json:"total_cost_usd"`
	DurationMs            int64    `json:"duration_ms"`
	Turns                 int64    `json:"num_turns"`
	Model                 string   `json:"model"`
}

func firstNonZero(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
