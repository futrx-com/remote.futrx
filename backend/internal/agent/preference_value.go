package agent

import "strings"

// NormalizePreferenceValue trims a user-selected provider preference and
// rejects values outside the persisted preference grammar.
func NormalizePreferenceValue(value string) string {
	value, valid := normalizedPreferenceValue(value)
	if !valid {
		return ""
	}
	return value
}

// ValidPreferenceValue reports whether a user-selected provider preference is
// safe to persist after trimming. An empty value represents provider Auto.
func ValidPreferenceValue(value string) bool {
	_, valid := normalizedPreferenceValue(value)
	return valid
}

func normalizedPreferenceValue(value string) (string, bool) {
	value = strings.TrimSpace(value)
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' {
			continue
		}
		return "", false
	}
	return value, true
}
