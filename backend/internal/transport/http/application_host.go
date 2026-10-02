package httptransport

import (
	"net"
	"strings"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// ApplicationHost uses the manifest label and project slug.
func ApplicationHost(identity, subdomain, publicHost string) string {
	if publicHost == "" || !validProjectApplicationLabel(subdomain, identity) {
		return ""
	}
	return subdomain + "--" + identity + "." + publicHost
}

// validProjectApplicationLabel owns the combined app/project DNS-label rules.
func validProjectApplicationLabel(label, slug string) bool {
	return label != "" && slug != "" && len(label)+2+len(slug) <= 63 &&
		svc.ValidWebSubdomain(label) && svc.ValidWebSubdomain(slug)
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
	host, base := requestHostname(host), requestHostname(publicHost)
	if base == "" || !strings.HasSuffix(host, "."+base) {
		return false
	}
	prefix := strings.TrimSuffix(host, "."+base)
	labels := strings.Split(prefix, ".")
	// Keep the existing preview namespace with its handler.
	last := labels[len(labels)-1]
	return last != "dev"
}

// ApplicationProject returns the manifest label and project slug of a named host.
func ApplicationProject(host, publicHost string) (string, string, bool) {
	if !IsApplicationHost(host, publicHost) {
		return "", "", false
	}
	name := strings.TrimSuffix(requestHostname(host), "."+requestHostname(publicHost))
	label, slug, found := strings.Cut(name, "--")
	if !found || !validProjectApplicationLabel(label, slug) {
		return "", "", false
	}
	return label, slug, true
}

func MatchesApplicationHost(host, identity, subdomain, publicHost string) bool {
	canonical := ApplicationHost(identity, subdomain, publicHost)
	return canonical != "" && requestHostname(host) == requestHostname(canonical)
}
