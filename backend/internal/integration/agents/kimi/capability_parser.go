package kimi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type rawObject map[string]json.RawMessage

func parseProviderCatalog(raw []byte, help, defaults string) (agent.Capabilities, error) {
	models, err := parseProviderModels(raw, parseDefaultModel(defaults))
	if err != nil {
		return agent.Capabilities{}, err
	}
	return agent.Capabilities{
		Provider:    agent.ProviderKimi,
		Label:       "Kimi",
		Source:      agent.CapabilitySourceLive,
		Models:      agent.WithAutoModel(models, "Kimi default"),
		Modes:       agent.ProviderModes(strings.Contains(help, "--plan")),
		DefaultMode: agent.RunModeDefault,
	}, nil
}

func parseProviderModels(raw []byte, configuredDefault string) ([]agent.ModelCapability, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	var root rawObject
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("decode kimi provider catalog: %w", err)
	}
	modelsRaw := root["models"]
	if len(modelsRaw) == 0 || string(modelsRaw) == "null" {
		return nil, nil
	}
	globalDefault := rawString(root, "default_model", "defaultModel")
	if configuredDefault != "" {
		globalDefault = configuredDefault
	}

	var byAlias map[string]json.RawMessage
	if err := json.Unmarshal(modelsRaw, &byAlias); err == nil {
		modelsByAlias := make(map[string]json.RawMessage, len(byAlias))
		for alias, model := range byAlias {
			if safe := normalizeKimiModel(alias); safe != "" {
				modelsByAlias[safe] = model
			}
		}
		aliases := make([]string, 0, len(modelsByAlias))
		for alias := range modelsByAlias {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		models := make([]agent.ModelCapability, 0, len(aliases))
		for _, alias := range aliases {
			models = append(models, parseModel(alias, modelsByAlias[alias], globalDefault))
		}
		return models, nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal(modelsRaw, &items); err != nil {
		return nil, fmt.Errorf("decode kimi models: %w", err)
	}
	models := make([]agent.ModelCapability, 0, len(items))
	seen := make(map[string]bool)
	for _, item := range items {
		var object rawObject
		if json.Unmarshal(item, &object) != nil {
			continue
		}
		alias := normalizeKimiModel(rawString(object, "alias", "id", "model"))
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		models = append(models, parseModel(alias, item, globalDefault))
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func parseModel(
	alias string,
	raw json.RawMessage,
	globalDefault string,
) agent.ModelCapability {
	var object rawObject
	_ = json.Unmarshal(raw, &object)
	var overrides rawObject
	if overrideRaw := object["overrides"]; len(overrideRaw) > 0 {
		_ = json.Unmarshal(overrideRaw, &overrides)
	}
	value := func(keys ...string) string {
		if result := rawString(overrides, keys...); result != "" {
			return result
		}
		return rawString(object, keys...)
	}
	values := func(keys ...string) []string {
		if result, exists := rawStringList(overrides, keys...); exists {
			return result
		}
		result, _ := rawStringList(object, keys...)
		return result
	}

	providerModel := strings.TrimSpace(value("model"))
	displayName := value("display_name", "displayName")
	if displayName == "" {
		displayName = providerModel
	}
	if displayName == "" {
		displayName = alias
	}
	description := value("description")
	if providerModel != "" && !strings.EqualFold(providerModel, displayName) {
		modelDescription := "Provider model: " + providerModel
		if description == "" {
			description = modelDescription
		} else {
			description += " · " + modelDescription
		}
	}

	reasoning := []agent.CapabilityOption{}
	for _, effort := range values("support_efforts", "supportEfforts") {
		effort = agent.NormalizeCapabilityValue(effort)
		if effort == "" {
			continue
		}
		if len(reasoning) == 0 {
			reasoning = append(reasoning, agent.AutoOption())
		}
		reasoning = append(reasoning, agent.CapabilityOption{
			Value: effort,
			Label: capabilityLabel(effort),
		})
	}
	defaultEffort := agent.NormalizeCapabilityValue(value("default_effort", "defaultEffort"))
	if !hasCapabilityOption(reasoning, defaultEffort) {
		defaultEffort = ""
	}
	return agent.ModelCapability{
		ID:                     alias,
		Label:                  displayName,
		Description:            description,
		ProviderDefault:        alias == globalDefault,
		ReasoningEfforts:       reasoning,
		DefaultReasoningEffort: defaultEffort,
	}
}

func hasCapabilityOption(options []agent.CapabilityOption, value string) bool {
	if value == "" {
		return false
	}
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func rawString(object rawObject, keys ...string) string {
	for _, key := range keys {
		raw := object[key]
		if len(raw) == 0 {
			continue
		}
		var value string
		if json.Unmarshal(raw, &value) == nil {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func parseDefaultModel(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Default model:") {
			continue
		}
		return normalizeKimiModel(strings.TrimSpace(strings.TrimPrefix(line, "Default model:")))
	}
	return ""
}

func rawStringList(object rawObject, keys ...string) ([]string, bool) {
	for _, key := range keys {
		raw := object[key]
		if len(raw) == 0 {
			continue
		}
		var values []string
		if json.Unmarshal(raw, &values) == nil {
			return values, true
		}
	}
	return nil, false
}

func normalizeKimiModel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 {
		return ""
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return value
}

func capabilityLabel(value string) string {
	if strings.EqualFold(value, "xhigh") {
		return "XHigh"
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '-' || r == '_' })
	for index, part := range parts {
		if part != "" {
			parts[index] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	if len(parts) == 0 {
		return value
	}
	return strings.Join(parts, " ")
}
