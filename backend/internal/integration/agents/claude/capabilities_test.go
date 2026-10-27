package claude

import (
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestParseModelCatalogResultReturnsEverySelection(t *testing.T) {
	result := "Current model: `Opus 4.8 (1M context) (default)`\n" +
		"Usage: /model <name>. Available: sonnet, opus, haiku, fable, best, " +
		"sonnet[1m], opus[1m], fable[1m], opusplan, default, or a full model ID."

	defaultLabel, selections, err := parseModelCatalogResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if defaultLabel != "Opus 4.8 (1M context)" {
		t.Fatalf("default label = %q", defaultLabel)
	}
	want := []string{
		"sonnet", "opus", "haiku", "fable", "best",
		"sonnet[1m]", "opus[1m]", "fable[1m]", "opusplan",
	}
	if !slices.Equal(selections, want) {
		t.Fatalf("selections = %#v, want %#v", selections, want)
	}
}

func TestCurrentModelLabelStripsInlineCodeFormatting(t *testing.T) {
	tests := []struct {
		name   string
		result string
		want   string
	}{
		{
			name:   "default marker inside backticks",
			result: "Current model: `Opus 5 (1M context) (default)`",
			want:   "Opus 5 (1M context)",
		},
		{
			name:   "explicit selection",
			result: "Current model: `Sonnet 5`",
			want:   "Sonnet 5",
		},
		{
			name:   "default marker outside backticks",
			result: "Current model: `Haiku 4.5` (default)",
			want:   "Haiku 4.5",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := currentModelLabel(test.result); got != test.want {
				t.Fatalf("currentModelLabel() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseEffortCommandOptionsIncludesUltracode(t *testing.T) {
	got := parseEffortCommandOptions(
		"Usage: /effort <low|medium|high|xhigh|max|ultracode|auto>",
	)
	want := []string{"", "low", "medium", "high", "xhigh", "max", "ultracode"}
	values := make([]string, 0, len(got))
	for _, option := range got {
		values = append(values, option.Value)
	}
	if !slices.Equal(values, want) {
		t.Fatalf("effort values = %v, want %v", values, want)
	}
}

func TestBuildCapabilitiesUsesResolvedVersionedLabels(t *testing.T) {
	catalog := claudeModelCatalog{
		Source:       agent.CapabilitySourceLive,
		DefaultLabel: "Opus 4.8 (1M context)",
		Selections: []claudeModelSelection{
			{ID: "fable", Label: "Fable 5", Description: "Claude Code selection: fable"},
			{ID: "opus", Label: "Opus 4.8", Description: "Claude Code selection: opus"},
			{ID: "opus[1m]", Label: "Opus 4.8 (1M context)", Description: "Claude Code selection: opus[1m]"},
		},
	}
	reasoning := parseEffortCommandOptions(
		"Usage: /effort <low|medium|high|xhigh|max|ultracode|auto>",
	)
	caps := buildCapabilities(catalog, reasoning)

	if len(caps.Models) != 4 || caps.Models[0].Label != "Auto · Opus 4.8 (1M context)" {
		t.Fatalf("models = %+v", caps.Models)
	}
	if caps.Models[1].ID != "fable" || caps.Models[1].Label != "Fable 5" {
		t.Fatalf("fable = %+v", caps.Models[1])
	}
	if got := caps.Models[1].ServiceTiers; len(got) != 0 {
		t.Fatalf("fable speed tiers = %+v", got)
	}
	for _, index := range []int{0, 2, 3} {
		if got := caps.Models[index].ServiceTiers; len(got) != 2 || got[1].Value != fastServiceTier {
			t.Fatalf("model %d speed tiers = %+v", index, got)
		}
	}
	if got := caps.Models[2].ReasoningEfforts; len(got) != 7 || got[5].Value != "max" || got[6].Value != ultracodeEffort {
		t.Fatalf("reasoning efforts = %+v", got)
	}
	if len(caps.Modes) != 2 || caps.Modes[0].Value != string(agent.RunModeDefault) || caps.Modes[1].Value != string(agent.RunModePlan) {
		t.Fatalf("modes = %+v", caps.Modes)
	}
}

func TestBuildCapabilitiesScopesEffortAndUltracodeByModel(t *testing.T) {
	catalog := claudeModelCatalog{
		Source:       agent.CapabilitySourceLive,
		DefaultLabel: "Opus 4.6",
		Selections: []claudeModelSelection{
			{ID: "opus", Label: "Opus 4.8"},
			{ID: "sonnet", Label: "Sonnet 4.6"},
			{ID: "haiku", Label: "Haiku 4.5"},
			{ID: "opusplan", Label: "Opus 5 (Plan) · Sonnet 5 (Default)"},
		},
	}
	reasoning := parseEffortCommandOptions(
		"Usage: /effort <low|medium|high|xhigh|max|ultracode|auto>",
	)
	caps := buildCapabilities(catalog, reasoning)

	assertEffortValues(t, caps.Models[0], []string{"", "low", "medium", "high", "max"})
	assertEffortValues(t, caps.Models[1], []string{"", "low", "medium", "high", "xhigh", "max", "ultracode"})
	assertEffortValues(t, caps.Models[2], []string{"", "low", "medium", "high", "max"})
	assertEffortValues(t, caps.Models[3], []string{})
	assertEffortValues(t, caps.Models[4], []string{"", "low", "medium", "high", "xhigh", "max", "ultracode"})
}

func TestReasoningOptionsDoNotTrustProviderWideUltracodeForUnknownModel(t *testing.T) {
	reasoning := parseHelpEfforts(
		"--effort <level> (low, medium, high, xhigh, max, ultracode)",
	)
	got := reasoningOptionsForModel(reasoning, "Custom gateway model")

	values := make([]string, 0, len(got))
	for _, option := range got {
		values = append(values, option.Value)
	}
	if slices.Contains(values, ultracodeEffort) {
		t.Fatalf("unknown model efforts = %v, want ultracode omitted", values)
	}
}

func assertEffortValues(t *testing.T, model agent.ModelCapability, want []string) {
	t.Helper()
	got := make([]string, 0, len(model.ReasoningEfforts))
	for _, option := range model.ReasoningEfforts {
		got = append(got, option.Value)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("model %q reasoning efforts = %v, want %v", model.Label, got, want)
	}
}

func TestFallbackCapabilitiesKeepCompleteVersionedCatalog(t *testing.T) {
	caps := fallbackCapabilities()
	if caps.Source != agent.CapabilitySourceFallback || len(caps.Models) != 10 {
		t.Fatalf("capabilities = %+v", caps)
	}
	want := map[string]string{
		"fable":    "Fable 5",
		"opus":     "Opus 5",
		"sonnet":   "Sonnet 5",
		"haiku":    "Haiku 4.5",
		"opus[1m]": "Opus 5 (1M context)",
		"opusplan": "Opus 5 (Plan) · Sonnet 5 (Default)",
	}
	for _, model := range caps.Models {
		if label, ok := want[model.ID]; ok && model.Label != label {
			t.Fatalf("model %q label = %q, want %q", model.ID, model.Label, label)
		}
	}
}
