package kimicodex

import (
	"encoding/json"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

// This is the Codex model metadata documented in Kimi's Codex guide. Model
// identity, context limits, modalities and effort levels come from /models.
type runtimeModel struct {
	Slug                          string                `json:"slug"`
	DisplayName                   string                `json:"display_name"`
	Description                   string                `json:"description"`
	DefaultReasoningLevel         agent.ReasoningEffort `json:"default_reasoning_level"`
	SupportedReasoningLevels      []runtimeEffort       `json:"supported_reasoning_levels"`
	ShellType                     string                `json:"shell_type"`
	ApplyPatchToolType            string                `json:"apply_patch_tool_type"`
	Visibility                    string                `json:"visibility"`
	SupportedInAPI                bool                  `json:"supported_in_api"`
	Priority                      int                   `json:"priority"`
	BaseInstructions              string                `json:"base_instructions"`
	SupportsReasoningSummaries    bool                  `json:"supports_reasoning_summaries"`
	DefaultReasoningSummary       string                `json:"default_reasoning_summary"`
	SupportVerbosity              bool                  `json:"support_verbosity"`
	TruncationPolicy              runtimeTruncation     `json:"truncation_policy"`
	ContextWindow                 int                   `json:"context_window"`
	MaxContextWindow              int                   `json:"max_context_window"`
	EffectiveContextWindowPercent int                   `json:"effective_context_window_percent"`
	SupportsParallelToolCalls     bool                  `json:"supports_parallel_tool_calls"`
	ExperimentalSupportedTools    []string              `json:"experimental_supported_tools"`
	InputModalities               []string              `json:"input_modalities"`
}

type runtimeEffort struct {
	Effort      agent.ReasoningEffort `json:"effort"`
	Description string                `json:"description"`
}

type runtimeTruncation struct {
	Mode  string `json:"mode"`
	Limit int    `json:"limit"`
}

func buildRuntimeModelCatalog(models []modelInfo) ([]byte, error) {
	if len(models) == 0 {
		return nil, ErrKimiCodexModelDiscoveryUnavailable
	}
	catalog := make([]runtimeModel, 0, len(models))
	for i, model := range models {
		if agent.NormalizeModelID(model.ID) != model.ID || !isCodingModel(model.ID) || model.ContextWindow <= 0 {
			return nil, ErrKimiCodexModelDiscoveryUnavailable
		}
		levels := make([]runtimeEffort, 0, len(model.Efforts))
		for _, effort := range model.Efforts {
			levels = append(levels, runtimeEffort{Effort: effort, Description: string(effort) + " reasoning"})
		}
		modalities := []string{"text"}
		if model.ImageInput {
			modalities = append(modalities, "image")
		}
		catalog = append(catalog, runtimeModel{
			Slug: model.ID, DisplayName: model.Label, Description: "Kimi Code",
			DefaultReasoningLevel: model.DefaultEffort, SupportedReasoningLevels: levels,
			ShellType: "shell_command", ApplyPatchToolType: "freeform",
			Visibility: "list", SupportedInAPI: true, Priority: i,
			BaseInstructions:           "You are Kimi, a coding agent powered by " + model.ID + ".",
			SupportsReasoningSummaries: true, DefaultReasoningSummary: "none",
			TruncationPolicy: runtimeTruncation{Mode: "bytes", Limit: 10_000},
			ContextWindow:    model.ContextWindow, MaxContextWindow: model.ContextWindow,
			EffectiveContextWindowPercent: 95, SupportsParallelToolCalls: true,
			ExperimentalSupportedTools: []string{}, InputModalities: modalities,
		})
	}
	return json.Marshal(struct {
		Models []runtimeModel `json:"models"`
	}{Models: catalog})
}

func kimiCodexRuntimeCatalogAsset(content []byte) provisioning.RuntimeAsset {
	return provisioning.RuntimeAsset{
		Content: content, Path: configconstants.KimiCodexContainerCatalog,
		HashPath:      configconstants.KimiCodexContainerCatalogHash,
		Mode:          configconstants.DefaultRuntimeAssetFileMode,
		Directory:     configconstants.KimiCodexContainerHome,
		DirectoryMode: configconstants.DefaultRuntimeAssetDirectoryMode,
	}
}
