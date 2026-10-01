package httptransport

import (
	"net"
	"regexp"
	"strings"
)

var applicationInstanceLabel = regexp.MustCompile(`^[a-f0-9]{12}$`)

// ApplicationHost uses installation IDs: concatenated project/app names can be
// ambiguous or exceed a DNS label. Reinstalls also get a fresh browser origin.
func ApplicationHost(instanceID, publicHost string) string {
	if !applicationInstanceLabel.MatchString(instanceID) || publicHost == "" {
		return ""
	}
	return instanceID + ".apps." + publicHost
}

func requestHostname(host string) string {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	// DNS treats a trailing dot as equivalent. Reserve that spelling too so
	// an app hostname can never fall through to the platform path router.
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

// IsApplicationHost also reserves malformed names, so they never fall through
// to the platform API, login pages, or static UI on an application origin.
func IsApplicationHost(host, publicHost string) bool {
	base := "apps." + requestHostname(publicHost)
	host = requestHostname(host)
	return publicHost != "" && (host == base || strings.HasSuffix(host, "."+base))
}

func ApplicationInstanceID(host, publicHost string) (string, bool) {
	if !IsApplicationHost(host, publicHost) {
		return "", false
	}
	id := strings.TrimSuffix(requestHostname(host), ".apps."+requestHostname(publicHost))
	return id, applicationInstanceLabel.MatchString(id)
}
