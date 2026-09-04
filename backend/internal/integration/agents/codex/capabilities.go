package codex

import (
	"context"
	"encoding/json"
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
	InputModalities           []string              `json:"inputModalities"`
	SupportsPersonality       bool                  `json:"supportsPersonality"`
	MultiAgentVersion         string                `json:"multiAgentVersion"`
	Hidden                    bool                  `json:"hidden"`
	ModelSpecialty            string                `json:"modelSpecialty"`
	Upgrade                   string                `json:"upgrade"`
	UpgradeInfo               json.RawMessage       `json:"upgradeInfo"`
	AvailabilityNux           json.RawMessage       `json:"availabilityNux"`
	Raw                       json.RawMessage       `json:"-"`
}

func (item *modelListItem) UnmarshalJSON(data []byte) error {
	type alias modelListItem
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*item = modelListItem(decoded)
	item.Raw = cloneRaw(data)
	return nil
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
	Name            string          `json:"name"`
	Mode            string          `json:"mode"`
	Model           string          `json:"model"`
	ReasoningEffort string          `json:"reasoning_effort"`
	Raw             json.RawMessage `json:"-"`
}

func (item *collaborationModeItem) UnmarshalJSON(data []byte) error {
	type alias collaborationModeItem
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*item = collaborationModeItem(decoded)
	item.Raw = cloneRaw(data)
	return nil
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
			InputModalities:        append([]string(nil), raw.InputModalities...),
			SupportsPersonality:    raw.SupportsPersonality,
			MultiAgentVersion:      raw.MultiAgentVersion,
			Hidden:                 raw.Hidden,
			ModelSpecialty:         raw.ModelSpecialty,
			Upgrade:                raw.Upgrade,
			UpgradeInfo:            cloneRaw(raw.UpgradeInfo),
			AvailabilityNux:        cloneRaw(raw.AvailabilityNux),
			Raw:                    cloneRaw(raw.Raw),
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
	modes := collaborationModeOptions(providerModes)
	return agent.Capabilities{
		Provider:    agent.ProviderCodex,
		Label:       "Codex",
		Source:      agent.CapabilitySourceLive,
		Models:      agent.WithAutoModel(items, "Codex default"),
		Modes:       modes,
		DefaultMode: agent.RunModeDefault,
	}
}

func collaborationModeOptions(providerModes collaborationModeListResponse) []agent.CapabilityOption {
	options := make([]agent.CapabilityOption, 0, len(providerModes.Data))
	seen := make(map[string]struct{}, len(providerModes.Data))
	for _, mode := range providerModes.Data {
		value := agent.NormalizeCapabilityValue(mode.Mode)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		options = append(options, agent.CapabilityOption{
			Value:           value,
			Label:           firstNonEmpty(mode.Name, effortLabel(value)),
			Model:           mode.Model,
			ReasoningEffort: mode.ReasoningEffort,
			Raw:             cloneRaw(mode.Raw),
		})
	}
	if len(options) == 0 {
		return agent.ProviderModes(false)
	}
	return options
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

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}
