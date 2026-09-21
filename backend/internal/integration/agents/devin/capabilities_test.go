package devin

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestParseModelCatalogFromArray(t *testing.T) {
	caps, err := parseModelCatalog([]byte(`[
		{"id":"devin-pro","name":"Devin Pro","description":"Pro model"},
		{"id":"devin-lite","name":"Devin Lite"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if caps.Provider != agent.ProviderDevin || caps.Label != "Devin" {
		t.Fatalf("caps identity = %#v", caps)
	}
	if caps.Source != agent.CapabilitySourceLive {
		t.Fatalf("source = %q, want live", caps.Source)
	}
	// Auto model prepended + 2 models = 3
	if len(caps.Models) != 3 {
		t.Fatalf("models = %#v", caps.Models)
	}
	if caps.Models[0].ID != "" || caps.Models[0].Label != "Auto" {
		t.Fatalf("auto model = %#v", caps.Models[0])
	}
	// Models are sorted by ID: devin-lite, devin-pro
	if caps.Models[1].ID != "devin-lite" || caps.Models[1].Label != "Devin Lite" {
		t.Fatalf("first model = %#v", caps.Models[1])
	}
	if caps.Models[2].ID != "devin-pro" || caps.Models[2].Label != "Devin Pro" {
		t.Fatalf("second model = %#v", caps.Models[2])
	}
	if caps.Models[2].Description != "Pro model" {
		t.Fatalf("description = %q", caps.Models[2].Description)
	}
}

func TestParseModelCatalogFromObjectWithModels(t *testing.T) {
	caps, err := parseModelCatalog([]byte(`{
		"models": [
			{"id":"devin-pro","name":"Devin Pro"},
			{"id":"devin-lite","name":"Devin Lite"}
		],
		"default": "devin-pro"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	// Auto + 2 models = 3
	if len(caps.Models) != 3 {
		t.Fatalf("models = %#v", caps.Models)
	}
	// devin-pro should be marked as provider default
	found := false
	for _, model := range caps.Models {
		if model.ID == "devin-pro" && model.ProviderDefault {
			found = true
		}
	}
	if !found {
		t.Fatal("devin-pro not marked as provider default")
	}
}

func TestParseModelCatalogFromDevinNativeFamilies(t *testing.T) {
	caps, err := parseModelCatalog([]byte(`{
		"families": [
			{
				"family_label": "Claude Opus 5",
				"family_uid": "claude-opus-5",
				"variants": [
					{"model_uid": "claude-opus-5-medium", "label": "Claude Opus 5 Medium"},
					{"model_uid": "claude-opus-5-low", "label": "Claude Opus 5 Low"}
				]
			},
			{
				"family_label": "GPT-5.2",
				"family_uid": "gpt-5.2",
				"variants": [
					{"model_uid": "MODEL_GPT_5_2_NONE", "label": "GPT-5.2 No Thinking"}
				]
			}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if caps.Source != agent.CapabilitySourceLive {
		t.Fatalf("source = %q, want live", caps.Source)
	}
	// Auto + 3 models = 4
	if len(caps.Models) != 4 {
		t.Fatalf("models = %#v, want 4 (auto + 3)", caps.Models)
	}
	// Sorted by ID: MODEL_GPT_5_2_NONE, claude-opus-5-low, claude-opus-5-medium
	if caps.Models[1].ID != "MODEL_GPT_5_2_NONE" || caps.Models[1].Label != "GPT-5.2 No Thinking" {
		t.Fatalf("first model = %#v", caps.Models[1])
	}
	if caps.Models[2].ID != "claude-opus-5-low" || caps.Models[2].Label != "Claude Opus 5 Low" {
		t.Fatalf("second model = %#v", caps.Models[2])
	}
	if caps.Models[3].ID != "claude-opus-5-medium" || caps.Models[3].Label != "Claude Opus 5 Medium" {
		t.Fatalf("third model = %#v", caps.Models[3])
	}
}

func TestParseModelCatalogFromFamiliesWithEmptyVariants(t *testing.T) {
	caps, err := parseModelCatalog([]byte(`{
		"families": [
			{"family_label": "Empty", "variants": []},
			{"family_label": "Also Empty", "variants": null}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	// No models found — should return auto-only (fallback through empty path)
	if len(caps.Models) != 1 || caps.Models[0].ID != "" {
		t.Fatalf("models = %#v, want only auto model", caps.Models)
	}
}

func TestParseModelCatalogFromObjectWithData(t *testing.T) {
	caps, err := parseModelCatalog([]byte(`{
		"data": [
			{"name":"devin-pro","displayName":"Devin Pro"},
			{"name":"devin-lite","displayName":"Devin Lite"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(caps.Models) != 3 {
		t.Fatalf("models = %#v", caps.Models)
	}
}

func TestParseModelCatalogWithAlternativeFieldNames(t *testing.T) {
	caps, err := parseModelCatalog([]byte(`[
		{"model":"devin-pro","title":"Devin Pro"},
		{"value":"devin-lite","label":"Devin Lite"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(caps.Models) != 3 {
		t.Fatalf("models = %#v", caps.Models)
	}
	// Sorted: devin-lite, devin-pro
	if caps.Models[1].ID != "devin-lite" || caps.Models[1].Label != "Devin Lite" {
		t.Fatalf("first model = %#v", caps.Models[1])
	}
	if caps.Models[2].ID != "devin-pro" || caps.Models[2].Label != "Devin Pro" {
		t.Fatalf("second model = %#v", caps.Models[2])
	}
}

func TestParseModelCatalogEmptyReturnsFallback(t *testing.T) {
	caps, err := parseModelCatalog([]byte(``))
	if err != nil {
		t.Fatal(err)
	}
	if caps.Source != agent.CapabilitySourceFallback {
		t.Fatalf("source = %q, want fallback", caps.Source)
	}
	if len(caps.Models) != 1 || caps.Models[0].ID != "" {
		t.Fatalf("models = %#v", caps.Models)
	}
}

func TestParseModelCatalogEmptyArrayReturnsAutoOnly(t *testing.T) {
	caps, err := parseModelCatalog([]byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(caps.Models) != 1 || caps.Models[0].ID != "" {
		t.Fatalf("models = %#v, want only auto model", caps.Models)
	}
}

func TestParseModelCatalogInvalidJSONReturnsError(t *testing.T) {
	_, err := parseModelCatalog([]byte(`not valid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseModelCatalogDeduplicatesModels(t *testing.T) {
	caps, err := parseModelCatalog([]byte(`[
		{"id":"devin-pro","name":"Devin Pro"},
		{"id":"devin-pro","name":"Duplicate"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	// Auto + 1 unique model = 2
	if len(caps.Models) != 2 {
		t.Fatalf("models = %#v, want 2 (auto + 1 unique)", caps.Models)
	}
}

func TestParseModelCatalogIncludesPlanMode(t *testing.T) {
	caps, err := parseModelCatalog([]byte(`[{"id":"devin-pro","name":"Devin Pro"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(caps.Modes) != 2 {
		t.Fatalf("modes = %#v, want default + plan", caps.Modes)
	}
	if caps.Modes[0].Value != string(agent.RunModeDefault) {
		t.Fatalf("first mode = %q, want default", caps.Modes[0].Value)
	}
	if caps.Modes[1].Value != string(agent.RunModePlan) {
		t.Fatalf("second mode = %q, want plan", caps.Modes[1].Value)
	}
	if caps.DefaultMode != agent.RunModeDefault {
		t.Fatalf("default mode = %q", caps.DefaultMode)
	}
}

func TestFallbackCapabilities(t *testing.T) {
	caps := fallbackCapabilities()
	if caps.Provider != agent.ProviderDevin || caps.Label != "Devin" {
		t.Fatalf("identity = %#v", caps)
	}
	if caps.Source != agent.CapabilitySourceFallback {
		t.Fatalf("source = %q", caps.Source)
	}
	if len(caps.Models) != 1 || caps.Models[0].ID != "" || caps.Models[0].Label != "Auto" {
		t.Fatalf("models = %#v", caps.Models)
	}
	if caps.Models[0].Description != "Devin default" {
		t.Fatalf("auto model description = %q", caps.Models[0].Description)
	}
	if len(caps.Modes) != 2 {
		t.Fatalf("modes = %#v, want default + plan", caps.Modes)
	}
}

func TestIsNotLoggedInError(t *testing.T) {
	tests := []struct {
		err      string
		expected bool
	}{
		{"not logged in", true},
		{"not authenticated", true},
		{"no credentials found", true},
		{"command not found", false},
		{"", false},
	}
	for _, test := range tests {
		var err error
		if test.err != "" {
			err = &simpleError{test.err}
		}
		if got := isNotLoggedInError(err); got != test.expected {
			t.Fatalf("isNotLoggedInError(%q) = %t, want %t", test.err, got, test.expected)
		}
	}
}

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }
