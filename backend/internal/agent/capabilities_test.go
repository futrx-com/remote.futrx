package agent

import (
	"testing"
)

func TestNormalizeCapabilityValue(t *testing.T) {
	for input, want := range map[string]string{
		" xhigh ":   "xhigh",
		"priority":  "priority",
		"future.v2": "future.v2",
		"bad value": "",
		"bad;value": "",
	} {
		if got := NormalizeCapabilityValue(input); got != want {
			t.Fatalf("NormalizeCapabilityValue(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPreferenceValueGrammarPreservesStoredSelections(t *testing.T) {
	for input, want := range map[string]string{
		" Future.V2 ": "Future.V2",
		"Burst_2":     "Burst_2",
		"":            "",
		"bad value":   "",
		"future-速":    "",
	} {
		if got := NormalizePreferenceValue(input); got != want {
			t.Fatalf("NormalizePreferenceValue(%q) = %q, want %q", input, got, want)
		}
	}
	if !ValidPreferenceValue(" Auto.V2 ") || !ValidPreferenceValue("") {
		t.Fatal("expected trimmed and Auto preference values to remain valid")
	}
	if ValidPreferenceValue("bad value") || ValidPreferenceValue("future-速") {
		t.Fatal("expected unsafe preference values to remain invalid")
	}
}

func TestNormalizeModelIDAllowsNamespacedModels(t *testing.T) {
	for input, want := range map[string]string{
		" openai/gpt-5 ": "openai/gpt-5",
		"provider:model": "provider:model",
		"bad model":      "",
	} {
		if got := NormalizeModelID(input); got != want {
			t.Fatalf("NormalizeModelID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWithAutoModelUsesProviderDefaultControls(t *testing.T) {
	models := []ModelCapability{
		{ID: "fast", ReasoningEfforts: []CapabilityOption{{Value: "low", Label: "Low"}}},
		{
			ID: "default", ProviderDefault: true,
			ReasoningEfforts: []CapabilityOption{{Value: "high", Label: "High"}},
			ServiceTiers:     []CapabilityOption{{Value: "priority", Label: "Fast"}},
		},
	}
	got := WithAutoModel(models, "provider default")
	if len(got) != 3 || got[0].ID != "" || got[0].ReasoningEfforts[0].Value != "high" || got[0].ServiceTiers[0].Value != "priority" {
		t.Fatalf("unexpected auto model: %+v", got[0])
	}
}
