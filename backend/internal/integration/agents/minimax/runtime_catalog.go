package minimax

import (
	"encoding/json"
	"fmt"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

type runtimeModelCatalog struct {
	Models []runtimeModel `json:"models"`
}

type runtimeModel struct {
	Slug                       string                  `json:"slug"`
	DisplayName                string                  `json:"display_name"`
	Description                string                  `json:"description"`
	DefaultReasoningLevel      agent.ReasoningEffort   `json:"default_reasoning_level"`
	SupportedReasoningLevels   []runtimeReasoningLevel `json:"supported_reasoning_levels"`
	ShellType                  string                  `json:"shell_type"`
	Visibility                 string                  `json:"visibility"`
	SupportedInAPI             bool                    `json:"supported_in_api"`
	Priority                   int                     `json:"priority"`
	BaseInstructions           string                  `json:"base_instructions"`
	SupportsReasoningSummaries bool                    `json:"supports_reasoning_summaries"`
	DefaultReasoningSummary    string                  `json:"default_reasoning_summary"`
	SupportVerbosity           bool                    `json:"support_verbosity"`
	TruncationPolicy           runtimeTruncationPolicy `json:"truncation_policy"`
	SupportsParallelToolCalls  bool                    `json:"supports_parallel_tool_calls"`
	ExperimentalSupportedTools []string                `json:"experimental_supported_tools"`
	InputModalities            []string                `json:"input_modalities"`
}

type runtimeReasoningLevel struct {
	Effort      agent.ReasoningEffort `json:"effort"`
	Description string                `json:"description"`
}

type runtimeTruncationPolicy struct {
	Mode  string `json:"mode"`
	Limit int    `json:"limit"`
}

func buildRuntimeModelCatalog(modelIDs []string) ([]byte, error) {
	models := make([]runtimeModel, 0, len(modelIDs))
	for index, id := range modelIDs {
		if agent.NormalizeModelID(id) != id || !isMiniMaxLanguageModel(id) {
			return nil, fmt.Errorf("%w: %q", ErrMiniMaxModelDiscoveryUnavailable, id)
		}
		levels := []runtimeReasoningLevel{{
			Effort:      configconstants.MiniMaxReasoningAdaptive,
			Description: configconstants.MiniMaxReasoningAdaptiveDescription,
		}}
		if miniMaxSupportsThinkingToggle(id) {
			levels = append([]runtimeReasoningLevel{{
				Effort:      configconstants.MiniMaxReasoningDisabled,
				Description: configconstants.MiniMaxReasoningDisabledDescription,
			}}, levels...)
		}
		models = append(models, runtimeModel{
			Slug:                       id,
			DisplayName:                id,
			Description:                configconstants.MiniMaxLabel,
			DefaultReasoningLevel:      configconstants.MiniMaxReasoningAdaptive,
			SupportedReasoningLevels:   levels,
			ShellType:                  "shell_command",
			Visibility:                 "list",
			SupportedInAPI:             true,
			Priority:                   index,
			BaseInstructions:           "You are MiniMax, a coding agent based on " + id + ".",
			SupportsReasoningSummaries: true,
			DefaultReasoningSummary:    "none",
			SupportVerbosity:           false,
			TruncationPolicy:           runtimeTruncationPolicy{Mode: "bytes", Limit: 10_000},
			SupportsParallelToolCalls:  true,
			ExperimentalSupportedTools: []string{},
			InputModalities:            []string{"text", "image"},
		})
	}
	if len(models) == 0 {
		return nil, ErrMiniMaxModelDiscoveryUnavailable
	}
	return json.Marshal(runtimeModelCatalog{Models: models})
}

func miniMaxRuntimeCatalogAsset(content []byte) provisioning.RuntimeAsset {
	return provisioning.RuntimeAsset{
		Content:       content,
		Path:          configconstants.MiniMaxContainerCatalog,
		HashPath:      configconstants.MiniMaxContainerCatalogHash,
		Mode:          configconstants.DefaultRuntimeAssetFileMode,
		Directory:     configconstants.MiniMaxContainerHome,
		DirectoryMode: configconstants.DefaultRuntimeAssetDirectoryMode,
	}
}
