package minimax

import (
	"context"
	"errors"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestCapabilitiesExposeDiscoveredMiniMaxModels(t *testing.T) {
	provider := &Provider{
		apiKeys: miniMaxTestAPIKeys{key: "sk-cp-key"},
		models:  miniMaxTestModels{models: []string{"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.5-highspeed"}},
	}
	caps, err := provider.Capabilities(context.Background(), agent.CapabilityRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if caps.Provider != agent.ProviderMiniMax || caps.DefaultMode != agent.RunModeDefault ||
		caps.Source != agent.CapabilitySourceLive {
		t.Fatalf("capabilities = %#v", caps)
	}
	if len(caps.Models) != 4 || caps.Models[0].ID != "" || caps.Models[1].ID != "MiniMax-M3" ||
		caps.Models[2].ID != "MiniMax-M2.7" || caps.Models[3].ID != "MiniMax-M2.5-highspeed" {
		t.Fatalf("models = %#v", caps.Models)
	}
	if !caps.Models[1].ProviderDefault || caps.Models[2].ProviderDefault {
		t.Fatalf("provider defaults = %#v", caps.Models)
	}
	if caps.Models[1].DefaultReasoningEffort != "high" || len(caps.Models[1].ReasoningEfforts) != 3 ||
		caps.Models[1].ReasoningEfforts[1].Value != "none" || caps.Models[1].ReasoningEfforts[2].Value != "high" {
		t.Fatalf("M3 reasoning = %#v", caps.Models[1])
	}
	if caps.Models[2].DefaultReasoningEffort != "high" || len(caps.Models[2].ReasoningEfforts) != 0 {
		t.Fatalf("M2.7 reasoning = %#v", caps.Models[2])
	}
	if len(caps.Modes) != 2 || caps.Modes[1].Value != string(agent.RunModePlan) {
		t.Fatalf("modes = %#v", caps.Modes)
	}
}

func TestCapabilitiesReturnUnpinnedFallbackWhenDiscoveryFails(t *testing.T) {
	provider := &Provider{
		apiKeys: miniMaxTestAPIKeys{key: "sk-cp-key"},
		models:  miniMaxTestModels{err: ErrMiniMaxModelDiscoveryUnavailable},
	}
	caps, err := provider.Capabilities(context.Background(), agent.CapabilityRequest{})
	if !errors.Is(err, ErrMiniMaxModelDiscoveryUnavailable) {
		t.Fatalf("error = %v", err)
	}
	if caps.Source != agent.CapabilitySourceFallback || len(caps.Models) != 1 || caps.Models[0].ID != "" {
		t.Fatalf("fallback = %#v", caps)
	}
}

func TestMiniMaxReasoningEffortUsesPerModelSupport(t *testing.T) {
	tests := []struct {
		model string
		input agent.ReasoningEffort
		want  agent.ReasoningEffort
	}{
		{model: "MiniMax-M3", input: "", want: "high"},
		{model: "MiniMax-M3", input: "none", want: "none"},
		{model: "MiniMax-M3-highspeed", input: "NONE", want: "none"},
		{model: "MiniMax-M3", input: "medium", want: "high"},
		{model: "MiniMax-M2.7", input: "none", want: "high"},
	}
	for _, test := range tests {
		if got := miniMaxReasoningEffort(test.model, test.input); got != test.want {
			t.Fatalf("miniMaxReasoningEffort(%q, %q) = %q, want %q", test.model, test.input, got, test.want)
		}
	}
}

type miniMaxTestModels struct {
	models []string
	err    error
}

func (s miniMaxTestModels) Models(context.Context, string) ([]string, error) {
	return append([]string(nil), s.models...), s.err
}
