package agent

import (
	"encoding/json"
	"testing"
)

func TestParseUsageAcceptsProviderSpellings(t *testing.T) {
	cost := 0.4213
	tests := []struct {
		name string
		raw  string
		want Usage
		ok   bool
	}{
		{
			name: "claude result usage",
			raw:  `{"input_tokens":12,"output_tokens":34,"cache_read_input_tokens":56,"cache_creation_input_tokens":78}`,
			want: Usage{InputTokens: 12, OutputTokens: 34, CacheReadTokens: 56, CacheWriteTokens: 78},
			ok:   true,
		},
		{
			name: "codex native usage",
			raw:  `{"input_tokens":10,"cached_input_tokens":3,"cache_write_input_tokens":2,"output_tokens":4,"reasoning_output_tokens":2}`,
			// Parsing accepts native aliases, but the owning provider adapter
			// decides whether input is inclusive and applies normalization.
			want: Usage{InputTokens: 10, CacheReadTokens: 3, CacheWriteTokens: 2, OutputTokens: 4, ReasoningTokens: 2},
			ok:   true,
		},
		{
			name: "openai chat spelling",
			raw:  `{"prompt_tokens":5,"completion_tokens":7}`,
			want: Usage{InputTokens: 5, OutputTokens: 7},
			ok:   true,
		},
		{
			name: "normalized payload round trip",
			raw:  `{"input_tokens":1,"total_cost_usd":0.4213,"duration_ms":900,"num_turns":3,"model":"claude-sonnet-4-5"}`,
			want: Usage{InputTokens: 1, CostUSD: &cost, DurationMs: 900, Turns: 3, Model: "claude-sonnet-4-5"},
			ok:   true,
		},
		{name: "model only", raw: `{"model":"kimi-k2"}`, want: Usage{Model: "kimi-k2"}, ok: true},
		{name: "empty object", raw: `{}`, ok: false},
		{name: "absent", raw: ``, ok: false},
		{name: "not an object", raw: `"nope"`, ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := ParseUsage(json.RawMessage(test.raw))
			if ok != test.ok {
				t.Fatalf("ok = %t, want %t", ok, test.ok)
			}
			if !ok {
				return
			}
			if (got.CostUSD == nil) != (test.want.CostUSD == nil) {
				t.Fatalf("cost presence mismatch: got %v want %v", got.CostUSD, test.want.CostUSD)
			}
			if got.CostUSD != nil && *got.CostUSD != *test.want.CostUSD {
				t.Fatalf("cost = %v, want %v", *got.CostUSD, *test.want.CostUSD)
			}
			got.CostUSD, test.want.CostUSD = nil, nil
			if got != test.want {
				t.Fatalf("usage = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestUsageRawOmitsEmptyPayload(t *testing.T) {
	if raw := (Usage{}).Raw(); raw != nil {
		t.Fatalf("empty usage rendered %s, want nil", raw)
	}
	raw := Usage{InputTokens: 2, OutputTokens: 3, Model: "gpt-5-codex"}.Raw()
	if string(raw) != `{"schema_version":1,"input_tokens":2,"output_tokens":3,"model":"gpt-5-codex"}` {
		t.Fatalf("usage rendered %s", raw)
	}
}

func TestUsageTotalTokensSumsEveryBucket(t *testing.T) {
	usage := Usage{InputTokens: 1, OutputTokens: 2, CacheReadTokens: 4, CacheWriteTokens: 8, ReasoningTokens: 16}
	// The four billable buckets are disjoint. Reasoning tokens are a subset of
	// output tokens upstream, so they must not be double counted.
	if got := usage.TotalTokens(); got != 15 {
		t.Fatalf("total = %d, want 15", got)
	}
}

func TestNormalizeInclusiveInputSplitsCacheSubsets(t *testing.T) {
	usage := NormalizeInclusiveInput(Usage{
		InputTokens:      10,
		OutputTokens:     4,
		CacheReadTokens:  3,
		CacheWriteTokens: 2,
	})
	if usage.InputTokens != 5 || usage.TotalTokens() != 14 {
		t.Fatalf("usage = %#v, want 5 uncached and 14 total tokens", usage)
	}
}

func TestNormalizeInclusiveInputNeverProducesNegativeTokens(t *testing.T) {
	usage := NormalizeInclusiveInput(Usage{InputTokens: 2, CacheReadTokens: 3})
	if usage.InputTokens != 0 || usage.CacheReadTokens != 2 || usage.TotalTokens() != 2 {
		t.Fatalf("usage = %#v, want the cache subset clamped to 2", usage)
	}
}
