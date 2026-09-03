package kimi

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestParseCapabilitiesFromProviderJSON(t *testing.T) {
	caps, err := parseProviderCatalog([]byte(`{
  "providers": {"custom": {"defaultModel": "fast"}},
  "models": {
    "moonshot/kimi-k2": {
      "provider": "custom",
      "model": "kimi-k2-0711-preview",
      "displayName": "Kimi K2 0711 Preview"
    },
    "fast": {
      "provider": "custom",
      "model": "kimi-k2.5",
      "displayName": "Kimi K2.5",
      "supportEfforts": ["low", "medium", "high"],
      "defaultEffort": "medium"
    }
  }
}`), "--plan Start in plan mode", "Default model: fast")
	if err != nil {
		t.Fatal(err)
	}
	if len(caps.Models) != 3 || caps.Models[1].ID != "fast" || caps.Models[2].ID != "moonshot/kimi-k2" {
		t.Fatalf("models = %+v", caps.Models)
	}
	if caps.Models[1].Label != "Kimi K2.5" || !caps.Models[1].ProviderDefault {
		t.Fatalf("fast model = %+v", caps.Models[1])
	}
	if got := caps.Models[1].ReasoningEfforts; len(got) != 4 || got[3].Value != "high" || caps.Models[1].DefaultReasoningEffort != "medium" {
		t.Fatalf("reasoning efforts = %+v", got)
	}
	if caps.Models[2].Label != "Kimi K2 0711 Preview" || caps.Models[2].Description != "Provider model: kimi-k2-0711-preview" {
		t.Fatalf("versioned model = %+v", caps.Models[2])
	}
	if len(caps.Modes) != 2 || caps.Modes[0].Value != string(agent.RunModeDefault) || caps.Modes[1].Value != string(agent.RunModePlan) {
		t.Fatalf("modes = %+v", caps.Modes)
	}
}

func TestParseCapabilitiesAllowsEmptyBuiltInCatalog(t *testing.T) {
	caps, err := parseProviderCatalog([]byte(`{"providers":{},"models":{}}`), "", "")
	if err != nil || len(caps.Models) != 1 || caps.Models[0].ID != "" {
		t.Fatalf("capabilities = %+v, err = %v", caps, err)
	}
}

func TestParseDefaultModel(t *testing.T) {
	if got := parseDefaultModel("custom type=anthropic models=2\n\nDefault model: moonshot/kimi-k2\n"); got != "moonshot/kimi-k2" {
		t.Fatalf("default model = %q", got)
	}
}

func TestModelOverridesCanRemoveBaseEfforts(t *testing.T) {
	models, err := parseProviderModels([]byte(`{
  "models": {
    "exact-alias": {
      "provider": "custom",
      "model": "provider/exact-model-v2",
      "displayName": "Base Model",
      "supportEfforts": ["low", "high"],
      "defaultEffort": "high",
      "overrides": {"displayName": "Exact Model V2", "supportEfforts": []}
    }
  }
}`), "exact-alias")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].Label != "Exact Model V2" || len(models[0].ReasoningEfforts) != 0 || models[0].DefaultReasoningEffort != "" {
		t.Fatalf("model = %+v", models)
	}
}
