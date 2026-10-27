package applications

import (
	"regexp"
	"strings"
)

var webSubdomainLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidWebSubdomain requires one DNS label without the reserved -- separator.
func ValidWebSubdomain(label string) bool {
	return webSubdomainLabel.MatchString(label) && !strings.Contains(label, "--")
}
