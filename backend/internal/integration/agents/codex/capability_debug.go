package codex

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

type debugCatalog struct {
	Models []debugModelItem `json:"models"`
}

type debugModelItem struct {
	Slug                  string               `json:"slug"`
	DisplayName           string               `json:"display_name"`
	Description           string               `json:"description"`
	DefaultReasoningLevel string               `json:"default_reasoning_level"`
	Visibility            string               `json:"visibility"`
	SupportedReasoning    []debugReasoningItem `json:"supported_reasoning_levels"`
	ServiceTiers          []serviceTierItem    `json:"service_tiers"`
}

type debugReasoningItem struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

func queryDebugModels(ctx context.Context, req agent.CapabilityRequest) (modelListResponse, error) {
	cmd := agentruntime.NewCapabilityCommand(
		ctx,
		req,
		[]string{"HOME=/root", "CODEX_HOME=/root/.codex", "OPENAI_API_KEY="},
		"codex",
		"debug",
		"models",
	)
	output, err := cmd.Output()
	if err != nil {
		return modelListResponse{}, err
	}
	var debug debugCatalog
	if err := json.Unmarshal(output, &debug); err != nil {
		return modelListResponse{}, err
	}
	var result modelListResponse
	for _, model := range debug.Models {
		if model.Visibility != "" && model.Visibility != "list" {
			continue
		}
		item := modelListItem{
			ID: model.Slug, Model: model.Slug, DisplayName: model.DisplayName,
			Description: model.Description, DefaultReasoningEffort: model.DefaultReasoningLevel,
			IsDefault: len(result.Data) == 0,
		}
		for _, effort := range model.SupportedReasoning {
			item.SupportedReasoningEfforts = append(
				item.SupportedReasoningEfforts,
				reasoningEffortItem{ReasoningEffort: effort.Effort, Description: effort.Description},
			)
		}
		for _, tier := range model.ServiceTiers {
			item.ServiceTiers = append(item.ServiceTiers, serviceTierItem{
				ID: tier.ID, Name: tier.Name, Description: tier.Description,
			})
		}
		result.Data = append(result.Data, item)
	}
	if len(result.Data) == 0 {
		return modelListResponse{}, errors.New("codex debug catalog returned no visible models")
	}
	return result, nil
}
