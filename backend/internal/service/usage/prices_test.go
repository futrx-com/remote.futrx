package usage

import (
	"math"
	"testing"
)

func TestPriceTableLookupPrefersTheLongestMatch(t *testing.T) {
	table, err := DefaultPriceTable().Normalize()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		model string
		want  string
		found bool
	}{
		{model: "claude-sonnet-4-5-20250929", want: "claude-sonnet-4", found: true},
		{model: "claude-opus-4-5", want: "claude-opus-4", found: true},
		{model: "sonnet", want: "sonnet", found: true},
		{model: "gpt-5-codex", want: "gpt-5-codex", found: true},
		{model: "gpt-5.6-terra", want: "gpt-5.6-terra", found: true},
		{model: "gpt-5.6-sol", want: "gpt-5.6", found: true},
		{model: "gpt-5-mini", want: "gpt-5-mini", found: true},
		{model: "gpt-5", want: "gpt-5", found: true},
		{model: "kimi-k2-turbo-preview", want: "kimi-k2", found: true},
		{model: "gemini-3-pro-preview", want: "gemini-3-pro", found: true},
		{model: "some-unknown-llm", found: false},
		{model: "", found: false},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			price, ok := table.Lookup(test.model)
			if ok != test.found {
				t.Fatalf("found = %t, want %t", ok, test.found)
			}
			if ok && price.Match != test.want {
				t.Fatalf("matched %q, want %q", price.Match, test.want)
			}
		})
	}
}

func TestDefaultOpenAIPricesIncludeCacheWrites(t *testing.T) {
	table, err := DefaultPriceTable().Normalize()
	if err != nil {
		t.Fatal(err)
	}
	price, ok := table.Lookup("gpt-5.6-sol")
	if !ok {
		t.Fatal("missing GPT-5.6 price")
	}
	if price.InputPerMTok != 4 || price.CacheReadPerMTok != 0.4 ||
		price.CacheWritePerMTok != 5 || price.OutputPerMTok != 20 {
		t.Fatalf("unexpected GPT-5.6 prices: %+v", price)
	}
}

func TestPriceTableEstimate(t *testing.T) {
	table, err := PriceTable{
		Currency: "usd",
		Models: []ModelPrice{{
			Match:             "test-model",
			InputPerMTok:      3,
			OutputPerMTok:     15,
			CacheReadPerMTok:  0.3,
			CacheWritePerMTok: 3.75,
		}},
	}.Normalize()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name                                 string
		model                                string
		input, output, cacheRead, cacheWrite int64
		want                                 float64
		ok                                   bool
	}{
		{
			name:  "all buckets",
			model: "test-model", input: 1_000_000, output: 1_000_000,
			cacheRead: 1_000_000, cacheWrite: 1_000_000,
			want: 3 + 15 + 0.3 + 3.75, ok: true,
		},
		{
			name:  "fractional input only",
			model: "test-model", input: 1500,
			want: 0.0045, ok: true,
		},
		{
			name:  "unknown model is not free",
			model: "other-model", input: 1_000_000,
			ok: false,
		},
		{
			name:  "zero tokens is unknown, not zero cost",
			model: "test-model",
			ok:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := table.Estimate(test.model, test.input, test.output, test.cacheRead, test.cacheWrite)
			if ok != test.ok {
				t.Fatalf("ok = %t, want %t", ok, test.ok)
			}
			if !ok {
				return
			}
			if math.Abs(got-test.want) > 1e-9 {
				t.Fatalf("estimate = %v, want %v", got, test.want)
			}
		})
	}
}

func TestPriceTableNormalizeRejectsUnusableTables(t *testing.T) {
	tests := []struct {
		name  string
		table PriceTable
	}{
		{name: "no rows", table: PriceTable{}},
		{name: "empty match", table: PriceTable{Models: []ModelPrice{{Match: "  "}}}},
		{
			name:  "negative rate",
			table: PriceTable{Models: []ModelPrice{{Match: "x", InputPerMTok: -1}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.table.Normalize(); err == nil {
				t.Fatal("expected normalization to fail")
			}
		})
	}
}

func TestPriceTableNormalizeCleansRows(t *testing.T) {
	table, err := PriceTable{
		Models: []ModelPrice{
			{Match: " GPT-5 ", InputPerMTok: 1},
			{Match: "gpt-5", InputPerMTok: 99},
			{Match: "gpt-5-codex", InputPerMTok: 2},
		},
	}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if table.Currency != "USD" || table.Version != PriceTableVersion {
		t.Fatalf("unexpected header: %+v", table)
	}
	if len(table.Models) != 2 {
		t.Fatalf("expected duplicate match to be dropped, got %+v", table.Models)
	}
	if table.Models[0].Match != "gpt-5-codex" {
		t.Fatalf("expected longest match first, got %q", table.Models[0].Match)
	}
}
