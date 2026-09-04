package agent

import (
	"strings"
	"unicode"
)

// NormalizeCapabilityValue accepts future provider values without embedding a
// moving enum in Remote while still rejecting strings that are unsafe to place
// inside provider configuration arguments.
func NormalizeCapabilityValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return ""
	}
	return value
}

// NormalizeModelID is deliberately less restrictive than capability values:
// custom provider registries commonly use namespaced ids such as org/model or
// provider:model. Model ids are always passed as a distinct process argument.
func NormalizeModelID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-_.:/@", r) {
			continue
		}
		return ""
	}
	return value
}

func AutoOption() CapabilityOption {
	return CapabilityOption{Value: "", Label: "Auto"}
}

func ProviderModes(supportsPlan bool) []CapabilityOption {
	modes := []CapabilityOption{{
		Value:       string(RunModeDefault),
		Label:       "Default",
		Description: "Use the provider's normal agent behavior",
	}}
	if supportsPlan {
		modes = append(modes, CapabilityOption{
			Value:       string(RunModePlan),
			Label:       "Plan",
			Description: "Use the provider's native planning mode",
		})
	}
	return modes
}

// WithAutoModel prepends the provider-default selection. Its controls mirror
// the provider's declared default model, or the first model when none is
// marked as default, so dependent selectors remain usable while the CLI
// chooses the actual model.
func WithAutoModel(models []ModelCapability, description string) []ModelCapability {
	auto := ModelCapability{ID: "", Label: "Auto", Description: description}
	foundDefault := false
	for _, model := range models {
		if model.ProviderDefault {
			auto = autoModelCapability(auto, model)
			foundDefault = true
			break
		}
	}
	if !foundDefault && len(models) > 0 {
		auto = autoModelCapability(auto, models[0])
	}
	return append([]ModelCapability{auto}, models...)
}

func autoModelCapability(auto, source ModelCapability) ModelCapability {
	auto.ReasoningEfforts = cloneCapabilityOptions(source.ReasoningEfforts)
	auto.DefaultReasoningEffort = source.DefaultReasoningEffort
	auto.ServiceTiers = cloneCapabilityOptions(source.ServiceTiers)
	auto.DefaultServiceTier = source.DefaultServiceTier
	auto.InputModalities = append([]string(nil), source.InputModalities...)
	auto.SupportsPersonality = source.SupportsPersonality
	auto.MultiAgentVersion = source.MultiAgentVersion
	auto.ModelSpecialty = source.ModelSpecialty
	auto.Upgrade = source.Upgrade
	auto.UpgradeInfo = append([]byte(nil), source.UpgradeInfo...)
	auto.AvailabilityNux = append([]byte(nil), source.AvailabilityNux...)
	return auto
}
