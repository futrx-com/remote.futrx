package webpush

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

// ErrUnsafeEndpoint means a subscription attempted to direct the server to a
// local, private, reserved, or otherwise non-public network address.
var ErrUnsafeEndpoint = errors.New("unsafe web push endpoint")

func validateEndpointURL(endpoint string) error {
	if len(endpoint) > 2048 {
		return fmt.Errorf("%w: endpoint is too long", ErrUnsafeEndpoint)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" {
		return errors.New("push endpoint must be an absolute https URL")
	}
	if parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" {
		return fmt.Errorf("%w: endpoint contains unsupported URL components", ErrUnsafeEndpoint)
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("%w: localhost is not a push service", ErrUnsafeEndpoint)
	}
	if address, err := netip.ParseAddr(host); err == nil && !isPublicAddress(address) {
		return fmt.Errorf("%w: %s is not public", ErrUnsafeEndpoint, host)
	}
	return nil
}

var nonPublicPrefixes = []netip.Prefix{
	// Shared, benchmarking, documentation, protocol-assignment, and reserved
	// IPv4 ranges that are not valid public push-service destinations.
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	// Documentation, transition, and well-known NAT64 ranges can otherwise
	// hide or synthesize a non-public IPv4 destination.
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func isPublicAddress(address netip.Addr) bool {
	if !address.IsValid() || address.Zone() != "" {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() ||
		address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}
