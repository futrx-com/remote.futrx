package agent

import "strings"

// NormalizeProviderID canonicalizes a provider identifier without assuming it
// belongs to Remote's current built-in set. Catalog membership is a separate
// application-level decision.
func NormalizeProviderID(value string) ProviderID {
	return ProviderID(strings.ToLower(strings.TrimSpace(value)))
}

// ValidProviderID accepts stable lowercase identifiers suitable for API keys,
// map keys, route segments, and persisted session ownership.
func ValidProviderID(id ProviderID) bool {
	value := string(id)
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		char := value[index]
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			continue
		}
		return false
	}
	return true
}
