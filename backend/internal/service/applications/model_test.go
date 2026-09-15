package applications

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplicationJSONReportsCapabilitiesWithoutAType(t *testing.T) {
	application := Application{
		ID:      "complete",
		Name:    "Complete",
		Install: "infra/install.sh",
		Port:    Port{Internal: 8080},
	}

	raw, err := json.Marshal(application)
	if err != nil {
		t.Fatalf("marshal application: %v", err)
	}
	jsonText := string(raw)
	if strings.Contains(jsonText, `"type"`) {
		t.Fatalf("application JSON still exposes a type: %s", jsonText)
	}
	if !strings.Contains(jsonText, `"needsContainer":true`) || !strings.Contains(jsonText, `"needsPort":true`) {
		t.Fatalf("application JSON does not expose inferred capabilities: %s", jsonText)
	}
}
