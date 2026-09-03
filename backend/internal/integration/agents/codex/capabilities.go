package codex

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type modelListResponse struct {
	Data       []modelListItem `json:"data"`
	NextCursor string          `json:"nextCursor"`
}

type modelListItem struct {
	ID                        string                `json:"id"`
	Model                     string                `json:"model"`
	DisplayName               string                `json:"displayName"`
	Description               string                `json:"description"`
	DefaultReasoningEffort    string                `json:"defaultReasoningEffort"`
	DefaultServiceTier        string                `json:"defaultServiceTier"`
	IsDefault                 bool                  `json:"isDefault"`
	SupportedReasoningEfforts []reasoningEffortItem `json:"supportedReasoningEfforts"`
	ServiceTiers              []serviceTierItem     `json:"serviceTiers"`
}

type reasoningEffortItem struct {
	ReasoningEffort string `json:"reasoningEffort"`
	Description     string `json:"description"`
}

type serviceTierItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type collaborationModeListResponse struct {
	Data []collaborationModeItem `json:"data"`
}

type collaborationModeItem struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

func (p *Provider) Capabilities(ctx context.Context, req agent.CapabilityRequest) (agent.Capabilities, error) {
	models, modes, err := queryAppServerCapabilities(ctx, req)
	if err == nil {
		return buildCapabilities(models, modes), nil
	}

	// If app-server discovery fails, including on older builds without
	// model/list, the debug catalog preserves structured live model, effort, and
	// service-tier data but cannot report collaboration modes.
	debugModels, debugErr := queryDebugModels(ctx, req)
	if debugErr == nil {
		caps := buildCapabilities(debugModels, collaborationModeListResponse{})
		caps.Warning = "Codex mode discovery is unavailable in this CLI version"
		return caps, nil
	}

	caps := fallbackCapabilities()
	caps.Warning = "Codex capabilities could not be read from the CLI"
	return caps, fmt.Errorf("codex capability discovery: %w", errors.Join(err, debugErr))
}

func buildCapabilities(models modelListResponse, providerModes collaborationModeListResponse) agent.Capabilities {
	items := make([]agent.ModelCapability, 0, len(models.Data))
	for _, raw := range models.Data {
		id := strings.TrimSpace(raw.Model)
		if id == "" {
			id = strings.TrimSpace(raw.ID)
		}
		if id == "" {
			continue
		}
		model := agent.ModelCapability{
			ID:                     id,
			Label:                  firstNonEmpty(raw.DisplayName, id),
			Description:            raw.Description,
			ProviderDefault:        raw.IsDefault,
			DefaultReasoningEffort: raw.DefaultReasoningEffort,
			DefaultServiceTier:     raw.DefaultServiceTier,
		}
		if len(raw.SupportedReasoningEfforts) > 0 {
			model.ReasoningEfforts = append(model.ReasoningEfforts, agent.AutoOption())
		}
		for _, effort := range raw.SupportedReasoningEfforts {
			model.ReasoningEfforts = append(model.ReasoningEfforts, agent.CapabilityOption{
				Value: effort.ReasoningEffort, Label: effortLabel(effort.ReasoningEffort), Description: effort.Description,
			})
		}
		if len(raw.ServiceTiers) > 0 {
			model.ServiceTiers = append(model.ServiceTiers, agent.AutoOption())
		}
		for _, tier := range raw.ServiceTiers {
			model.ServiceTiers = append(model.ServiceTiers, agent.CapabilityOption{
				Value: tier.ID, Label: firstNonEmpty(tier.Name, tier.ID), Description: tier.Description,
			})
		}
		items = append(items, model)
	}
	nativePlan := false
	for _, mode := range providerModes.Data {
		if strings.EqualFold(mode.Mode, string(agent.RunModePlan)) {
			nativePlan = true
		}
	}
	return agent.Capabilities{
		Provider:    agent.ProviderCodex,
		Label:       "Codex",
		Source:      agent.CapabilitySourceLive,
		Models:      agent.WithAutoModel(items, "Codex default"),
		Modes:       agent.ProviderModes(nativePlan),
		DefaultMode: agent.RunModeDefault,
	}
}

func fallbackCapabilities() agent.Capabilities {
	return agent.Capabilities{
		Provider:    agent.ProviderCodex,
		Label:       "Codex",
		Source:      agent.CapabilitySourceFallback,
		Models:      agent.WithAutoModel(nil, "Codex default"),
		Modes:       agent.ProviderModes(false),
		DefaultMode: agent.RunModeDefault,
	}
}

func effortLabel(value string) string {
	if strings.EqualFold(value, "xhigh") {
		return "XHigh"
	}
	if value == "" {
		return "Auto"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
