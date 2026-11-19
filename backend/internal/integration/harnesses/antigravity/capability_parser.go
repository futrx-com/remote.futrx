package antigravity

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
)

var (
	cliEffortChoicesPattern = regexp.MustCompile(`(?im)--effort\s+.*?\(([^)]*)\)`)
	cliModeChoicesPattern   = regexp.MustCompile(`(?im)--mode\s+.*?\(([^)]*)\)`)
)

func parseCLIOutputCatalog(modelsOutput, help string) harness.Capabilities {
	efforts := parseCLIChoices(cliEffortChoicesPattern, help)
	reasoning := make([]harness.CapabilityOption, 0, len(efforts)+1)
	if len(efforts) > 0 {
		reasoning = append(reasoning, harness.AutoOption())
		for _, effort := range efforts {
			reasoning = append(reasoning, harness.CapabilityOption{Value: effort, Label: capabilityLabel(effort)})
		}
	}
	modelIDs := parseCLIModelIDs(modelsOutput)
	models := make([]harness.ModelCapability, 0, len(modelIDs))
	for _, id := range modelIDs {
		models = append(models, harness.ModelCapability{
			ID: id, Label: id, ReasoningEfforts: append([]harness.CapabilityOption(nil), reasoning...),
		})
	}
	modes := parseCLIChoices(cliModeChoicesPattern, help)
	return harness.Capabilities{
		Provider:    harness.ProviderAntigravity,
		Label:       "Antigravity",
		Source:      harness.CapabilitySourceLive,
		Models:      harness.WithAutoModel(models, "Antigravity default"),
		Modes:       harness.ProviderModes(containsCLIChoice(modes, string(harness.RunModePlan))),
		DefaultMode: harness.RunModeDefault,
	}
}

func parseCLIModelIDs(output string) []string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}
	var jsonObject struct {
		Models []struct {
			ID          string `json:"id"`
			Model       string `json:"model"`
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"models"`
	}
	if strings.HasPrefix(trimmed, "{") && json.Unmarshal([]byte(trimmed), &jsonObject) == nil {
		ids := make([]string, 0, len(jsonObject.Models))
		for _, model := range jsonObject.Models {
			id := firstCLIModelID(model.DisplayName, model.Name, model.Model, model.ID)
			if id != "" {
				ids = append(ids, id)
			}
		}
		return uniqueCLIModels(ids)
	}

	ids := make([]string, 0)
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if line == "" || strings.Contains(lower, "sign in") || strings.Contains(lower, "available model") ||
			strings.HasPrefix(lower, "usage") || strings.HasPrefix(lower, "flags") ||
			strings.EqualFold(line, "model") || strings.HasPrefix(line, "---") {
			continue
		}
		line = strings.TrimLeft(line, "*-•>✓ ")
		if id := normalizeCLIModel(line); id != "" {
			ids = append(ids, id)
		}
	}
	return uniqueCLIModels(ids)
}

func parseCLIChoices(pattern *regexp.Regexp, input string) []string {
	match := pattern.FindStringSubmatch(input)
	if len(match) < 2 {
		return nil
	}
	return uniqueCLIValues(strings.FieldsFunc(match[1], func(r rune) bool {
		return r == '|' || r == ',' || r == ' '
	}))
}

func uniqueCLIValues(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = harness.NormalizeCapabilityValue(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func uniqueCLIModels(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeCLIModel(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func firstCLIModelID(values ...string) string {
	for _, value := range values {
		if safe := normalizeCLIModel(value); safe != "" {
			return safe
		}
	}
	return ""
}

func normalizeCLIModel(value string) string {
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

func containsCLIChoice(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func capabilityLabel(value string) string {
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
