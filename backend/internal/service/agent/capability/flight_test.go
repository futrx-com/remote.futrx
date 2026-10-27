package capability

import (
	"encoding/json"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestCloneCapabilitiesIsolatesNestedProviderMetadata(t *testing.T) {
	input := []agent.Capabilities{{
		ExecutionScopes: []string{"host"},
		Modes: []agent.CapabilityOption{{
			Value: "default",
			Raw:   json.RawMessage(`{"mode":true}`),
		}},
		Models: []agent.ModelCapability{{
			ID:              "model",
			InputModalities: []string{"text"},
			Raw:             json.RawMessage(`{"model":true}`),
			UpgradeInfo:     json.RawMessage(`{"upgrade":true}`),
			AvailabilityNux: json.RawMessage(`{"available":true}`),
			ReasoningEfforts: []agent.CapabilityOption{{
				Value: "high",
				Raw:   json.RawMessage(`{"effort":true}`),
			}},
			ServiceTiers: []agent.CapabilityOption{{
				Value: "priority",
				Raw:   json.RawMessage(`{"tier":true}`),
			}},
		}},
	}}
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	cloned := cloneCapabilities(input)
	cloned[0].ExecutionScopes[0] = "project"
	cloned[0].Modes[0].Raw[0] = '['
	cloned[0].Models[0].InputModalities[0] = "image"
	cloned[0].Models[0].Raw[0] = '['
	cloned[0].Models[0].UpgradeInfo[0] = '['
	cloned[0].Models[0].AvailabilityNux[0] = '['
	cloned[0].Models[0].ReasoningEfforts[0].Raw[0] = '['
	cloned[0].Models[0].ServiceTiers[0].Raw[0] = '['

	after, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("clone mutated source metadata:\n before: %s\n  after: %s", before, after)
	}
}
