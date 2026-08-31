package opencode

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestParseModelsOutputReadsBareModelLines(t *testing.T) {
	caps, err := parseModelsOutput("anthropic/claude-sonnet-4-5\nopencode/big-pickle\n\nopenai/gpt-5\n")
	if err != nil {
		t.Fatal(err)
	}
	if caps.Source != agent.CapabilitySourceLive || caps.Provider != agent.ProviderOpenCode {
		t.Fatalf("caps header = %#v", caps)
	}
	if len(caps.Models) != 4 {
		t.Fatalf("models = %#v", caps.Models)
	}
	if caps.Models[1].ID != "anthropic/claude-sonnet-4-5" || caps.Models[3].ID != "openai/gpt-5" {
		t.Fatalf("model order = %#v", caps.Models)
	}
	if len(caps.Models) == 0 || caps.Models[0].ID != "" || caps.Models[0].Label != "Auto" {
		t.Fatalf("auto model option missing: %#v", caps.Models)
	}
}

func TestParseModelsOutputRejectsEmptyList(t *testing.T) {
	if _, err := parseModelsOutput("\n  \n"); err == nil {
		t.Fatal("expected error for empty model list")
	}
}

func TestFallbackCapabilitiesStayUsable(t *testing.T) {
	caps := fallbackCapabilities()
	if caps.Source != agent.CapabilitySourceFallback || caps.Label != "OpenCode" {
		t.Fatalf("fallback caps = %#v", caps)
	}
	if len(caps.Models) == 0 || caps.Models[0].ID != "" {
		t.Fatalf("fallback auto model missing: %#v", caps.Models)
	}
}
