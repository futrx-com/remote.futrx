package claude

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

var (
	effortPattern        = regexp.MustCompile(`(?is)--effort\s+<level>.*?\(([^)]*)\)`)
	effortCommandPattern = regexp.MustCompile(`(?is)/effort\s+<([^>]+)>`)
)

func parseHelpEfforts(help string) []agent.CapabilityOption {
	efforts := parseHelpChoiceValues(effortPattern, help)
	return reasoningOptions(efforts)
}

func parseEffortCommandOptions(output string) []agent.CapabilityOption {
	efforts := parseHelpChoiceValues(effortCommandPattern, output)
	return reasoningOptions(efforts)
}

func reasoningOptions(efforts []string) []agent.CapabilityOption {
	if len(efforts) == 0 {
		return nil
	}
	reasoning := []agent.CapabilityOption{agent.AutoOption()}
	for _, effort := range efforts {
		if effort == "auto" {
			continue
		}
		reasoning = append(reasoning, agent.CapabilityOption{
			Value: effort,
			Label: optionLabel(effort),
		})
	}
	return reasoning
}

func parseHelpChoiceValues(pattern *regexp.Regexp, input string) []string {
	match := pattern.FindStringSubmatch(input)
	if len(match) < 2 {
		return nil
	}
	parts := strings.FieldsFunc(match[1], func(r rune) bool {
		return r == ',' || r == '|' || unicode.IsSpace(r)
	})
	return uniqueHelpValues(parts)
}

func uniqueHelpValues(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = agent.NormalizeCapabilityValue(strings.Trim(value, "'\""))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func optionLabel(value string) string {
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
