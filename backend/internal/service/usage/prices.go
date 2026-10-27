package usage

import (
	"errors"
	"sort"
	"strings"
)

// PriceTableVersion is the schema version written to prices.json.
const PriceTableVersion = 1

var ErrInvalidPrice = errors.New("invalid price entry")

// ModelPrice is one row of the editable price table. Rates are US dollars per
// one million tokens, matching how every vendor publishes them.
type ModelPrice struct {
	// Match is a case-insensitive substring of the model id. The longest
	// matching entry wins, so "claude-opus-4-5" beats "claude".
	Match            string  `json:"match"`
	Label            string  `json:"label,omitempty"`
	InputPerMTok     float64 `json:"inputPerMTok"`
	OutputPerMTok    float64 `json:"outputPerMTok"`
	CacheReadPerMTok float64 `json:"cacheReadPerMTok,omitempty"`
	// CacheWritePerMTok prices cache creation. Providers that do not bill
	// cache writes separately leave it zero.
	CacheWritePerMTok float64 `json:"cacheWritePerMTok,omitempty"`
}

// PriceTable is the whole editable document stored at
// DATA_DIR/usage/prices.json.
type PriceTable struct {
	Version   int          `json:"version"`
	UpdatedAt int64        `json:"updatedAt,omitempty"`
	Currency  string       `json:"currency"`
	Models    []ModelPrice `json:"models"`
}

// DefaultPriceTable seeds prices.json on first use. The rates are published
// list prices per million tokens at the time of writing; they are a starting
// point for estimates, not a billing source of truth, and an administrator is
// expected to edit them.
func DefaultPriceTable() PriceTable {
	return PriceTable{
		Version:  PriceTableVersion,
		Currency: "USD",
		Models: []ModelPrice{
			// Anthropic — Claude Code reports exact cost, so these only
			// matter for runs whose cost the CLI omitted.
			{Match: "claude-opus-4", Label: "Claude Opus 4.x", InputPerMTok: 15, OutputPerMTok: 75, CacheReadPerMTok: 1.5, CacheWritePerMTok: 18.75},
			{Match: "claude-sonnet-4", Label: "Claude Sonnet 4.x", InputPerMTok: 3, OutputPerMTok: 15, CacheReadPerMTok: 0.3, CacheWritePerMTok: 3.75},
			{Match: "claude-haiku-4", Label: "Claude Haiku 4.x", InputPerMTok: 1, OutputPerMTok: 5, CacheReadPerMTok: 0.1, CacheWritePerMTok: 1.25},
			{Match: "claude-3-5-haiku", Label: "Claude Haiku 3.5", InputPerMTok: 0.8, OutputPerMTok: 4, CacheReadPerMTok: 0.08, CacheWritePerMTok: 1},
			{Match: "opus", Label: "Claude Opus (alias)", InputPerMTok: 15, OutputPerMTok: 75, CacheReadPerMTok: 1.5, CacheWritePerMTok: 18.75},
			{Match: "sonnet", Label: "Claude Sonnet (alias)", InputPerMTok: 3, OutputPerMTok: 15, CacheReadPerMTok: 0.3, CacheWritePerMTok: 3.75},
			{Match: "haiku", Label: "Claude Haiku (alias)", InputPerMTok: 1, OutputPerMTok: 5, CacheReadPerMTok: 0.1, CacheWritePerMTok: 1.25},

			// OpenAI — Codex reports tokens only. GPT-5.6 bills cache writes at
			// 1.25x ordinary input; earlier models use the ordinary input rate if
			// they ever report a write bucket.
			{Match: "gpt-5.6-terra", Label: "GPT-5.6 Terra", InputPerMTok: 2, OutputPerMTok: 12, CacheReadPerMTok: 0.2, CacheWritePerMTok: 2.5},
			{Match: "gpt-5.6-luna", Label: "GPT-5.6 Luna", InputPerMTok: 0.2, OutputPerMTok: 1.2, CacheReadPerMTok: 0.02, CacheWritePerMTok: 0.25},
			{Match: "gpt-5.6", Label: "GPT-5.6 Sol", InputPerMTok: 4, OutputPerMTok: 20, CacheReadPerMTok: 0.4, CacheWritePerMTok: 5},
			{Match: "gpt-5-codex", Label: "GPT-5 Codex", InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125, CacheWritePerMTok: 1.25},
			{Match: "gpt-5-mini", Label: "GPT-5 mini", InputPerMTok: 0.25, OutputPerMTok: 2, CacheReadPerMTok: 0.025, CacheWritePerMTok: 0.25},
			{Match: "gpt-5-nano", Label: "GPT-5 nano", InputPerMTok: 0.05, OutputPerMTok: 0.4, CacheReadPerMTok: 0.005, CacheWritePerMTok: 0.05},
			{Match: "gpt-5", Label: "GPT-5", InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125, CacheWritePerMTok: 1.25},

			// Moonshot — kimi-code emits no usage today, so this only applies
			// if a future CLI version starts reporting tokens.
			{Match: "kimi-k2", Label: "Kimi K2", InputPerMTok: 0.6, OutputPerMTok: 2.5, CacheReadPerMTok: 0.15},
			{Match: "kimi", Label: "Kimi (alias)", InputPerMTok: 0.6, OutputPerMTok: 2.5, CacheReadPerMTok: 0.15},

			// Google — Antigravity print mode emits no usage today.
			{Match: "gemini-3-pro", Label: "Gemini 3 Pro", InputPerMTok: 2, OutputPerMTok: 12, CacheReadPerMTok: 0.2},
			{Match: "gemini-2.5-pro", Label: "Gemini 2.5 Pro", InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125},
			{Match: "gemini", Label: "Gemini (alias)", InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125},
		},
	}
}

// Normalize repairs a table submitted by an administrator: it drops rows with
// no match key or negative rates, lower-cases match keys, and rejects a table
// that ends up empty so a bad edit cannot silently disable all estimation.
func (t PriceTable) Normalize() (PriceTable, error) {
	out := PriceTable{
		Version:   PriceTableVersion,
		UpdatedAt: t.UpdatedAt,
		Currency:  strings.ToUpper(strings.TrimSpace(t.Currency)),
	}
	if out.Currency == "" {
		out.Currency = "USD"
	}
	seen := map[string]bool{}
	for _, model := range t.Models {
		match := strings.ToLower(strings.TrimSpace(model.Match))
		if match == "" {
			return PriceTable{}, ErrInvalidPrice
		}
		if model.InputPerMTok < 0 || model.OutputPerMTok < 0 ||
			model.CacheReadPerMTok < 0 || model.CacheWritePerMTok < 0 {
			return PriceTable{}, ErrInvalidPrice
		}
		if seen[match] {
			continue
		}
		seen[match] = true
		model.Match = match
		model.Label = strings.TrimSpace(model.Label)
		out.Models = append(out.Models, model)
	}
	if len(out.Models) == 0 {
		return PriceTable{}, ErrInvalidPrice
	}
	// Longest match first so lookup can stop at the first hit.
	sort.SliceStable(out.Models, func(i, j int) bool {
		return len(out.Models[i].Match) > len(out.Models[j].Match)
	})
	return out, nil
}

// Lookup returns the most specific price row for a model id.
func (t PriceTable) Lookup(model string) (ModelPrice, bool) {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return ModelPrice{}, false
	}
	best := ModelPrice{}
	found := false
	for _, price := range t.Models {
		if !strings.Contains(model, price.Match) {
			continue
		}
		if !found || len(price.Match) > len(best.Match) {
			best = price
			found = true
		}
	}
	return best, found
}

// Estimate prices a run from the table. It returns false when the model is
// unknown or the run reported no tokens at all — an unknown cost must never
// be recorded as zero.
func (t PriceTable) Estimate(model string, inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int64) (float64, bool) {
	if inputTokens+outputTokens+cacheReadTokens+cacheWriteTokens == 0 {
		return 0, false
	}
	price, ok := t.Lookup(model)
	if !ok {
		return 0, false
	}
	const perMillion = 1_000_000.0
	cost := float64(inputTokens)*price.InputPerMTok +
		float64(outputTokens)*price.OutputPerMTok +
		float64(cacheReadTokens)*price.CacheReadPerMTok +
		float64(cacheWriteTokens)*price.CacheWritePerMTok
	return cost / perMillion, true
}
