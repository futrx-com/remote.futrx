package webpush

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"testing"
)

func TestPublicDialerRejectsPrivateAndMixedDNSAnswers(t *testing.T) {
	for _, addresses := range [][]netip.Addr{
		{netip.MustParseAddr("127.0.0.1")},
		{netip.MustParseAddr("10.0.0.4")},
		{netip.MustParseAddr("169.254.169.254")},
		{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("192.168.1.20")},
	} {
		dialed := false
		dialer := publicDialer{
			lookup: func(context.Context, string, string) ([]netip.Addr, error) { return addresses, nil },
			dial: func(context.Context, string, string) (net.Conn, error) {
				dialed = true
				return nil, errors.New("unexpected dial")
			},
		}
		_, err := dialer.DialContext(context.Background(), "tcp", "push.example.com:443")
		if !errors.Is(err, ErrUnsafeEndpoint) {
			t.Fatalf("DialContext() error = %v, want ErrUnsafeEndpoint for %v", err, addresses)
		}
		if dialed {
			t.Fatalf("DialContext() dialed an answer from %v", addresses)
		}
	}
}

func TestPublicDialerPinsTheValidatedDNSAnswer(t *testing.T) {
	wantAddress := netip.MustParseAddr("93.184.216.34")
	wantErr := errors.New("stop after observing address")
	var dialed string
	dialer := publicDialer{
		lookup: func(_ context.Context, network, host string) ([]netip.Addr, error) {
			if network != "ip" || host != "push.example.com" {
				t.Fatalf("lookup = %q %q", network, host)
			}
			return []netip.Addr{wantAddress}, nil
		},
		dial: func(_ context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" {
				t.Fatalf("network = %q", network)
			}
			dialed = address
			return nil, wantErr
		},
	}

	_, err := dialer.DialContext(context.Background(), "tcp", "push.example.com:443")
	if !errors.Is(err, wantErr) {
		t.Fatalf("DialContext() error = %v, want %v", err, wantErr)
	}
	if dialed != "93.184.216.34:443" {
		t.Fatalf("dialed address = %q", dialed)
	}
}

func TestSafeHTTPClientDisablesProxyAndRedirects(t *testing.T) {
	client := newSafeHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("safe push transport inherited an HTTP proxy")
	}
	if err := client.CheckRedirect(&http.Request{}, []*http.Request{{}}); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect() error = %v, want ErrUseLastResponse", err)
	}
}
