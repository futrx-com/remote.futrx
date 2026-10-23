package provisioning

import (
	_ "embed"
	"strings"
)

const publicHostnamePlaceholder = "{{PUBLIC_HOSTNAME}}"

//go:embed assets/AGENTS.md
var instructionsTemplate []byte

func InstructionsTemplate(publicHostname string) []byte {
	hostname := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(publicHostname)), ".")
	return []byte(strings.ReplaceAll(
		string(instructionsTemplate),
		publicHostnamePlaceholder,
		hostname,
	))
}

// ProjectInstructions renders project-owned routing metadata without mutating
// the shared template reused by other projects.
func ProjectInstructions(template []byte, slug string) []byte {
	return []byte(strings.ReplaceAll(string(template), "{{PROJECT_SLUG}}", slug))
}
