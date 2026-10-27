package webpush

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type lookupNetIPFunc func(context.Context, string, string) ([]netip.Addr, error)
type dialContextFunc func(context.Context, string, string) (net.Conn, error)

// publicDialer resolves a hostname once, validates every answer, then dials one
// of those exact IPs. Pinning the dial to the checked result prevents a second
// lookup from turning a public hostname into a private target (DNS rebinding).
type publicDialer struct {
	lookup lookupNetIPFunc
	dial   dialContextFunc
}

func newSafeHTTPClient() *http.Client {
	networkDialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	dialer := publicDialer{
		lookup: net.DefaultResolver.LookupNetIP,
		dial:   networkDialer.DialContext,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A configured HTTP proxy could perform its own unchecked DNS lookup and
	// bypass the pinned dialer, so push delivery always connects directly.
	transport.Proxy = nil
	transport.DialContext = dialer.DialContext

	return &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		// Push endpoints are opaque capabilities, not navigation URLs. Following
		// a redirect would let an otherwise public endpoint redirect into the
		// server's private network.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (d publicDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse web push address: %w", err)
	}

	addresses, err := d.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("resolve web push endpoint %q: no addresses", host)
	}
	// Reject the whole hostname when any answer is unsafe. Selecting only a
	// public answer would leave mixed-answer DNS rebinding tricks available.
	for _, address := range addresses {
		if !isPublicAddress(address) {
			return nil, fmt.Errorf("%w: %s resolves to %s", ErrUnsafeEndpoint, host, address)
		}
	}

	var dialErrors []error
	for _, resolved := range addresses {
		connection, err := d.dial(ctx, network, net.JoinHostPort(resolved.String(), port))
		if err == nil {
			return connection, nil
		}
		dialErrors = append(dialErrors, err)
	}
	return nil, fmt.Errorf("dial web push endpoint %q: %w", host, errors.Join(dialErrors...))
}

func (d publicDialer) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if address, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{address}, nil
	}
	addresses, err := d.lookup(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve web push endpoint %q: %w", host, err)
	}
	return addresses, nil
}
