package httphandlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The rate limiter that guards /auth/local/claim and /auth/local/login keys on
// localClientIP. Caddy fronts the backend on loopback and *appends* the peer
// address to any X-Forwarded-For the client supplied, so the rightmost entry is
// the only one our own proxy vouches for. Reading the leftmost entry instead
// hands the limiter's key to the caller: rotate the header, get a fresh bucket
// every request, and the limit never fires.

func TestLocalClientIPIgnoresSpoofedForwardedPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/local/claim", nil)
	req.RemoteAddr = "127.0.0.1:41234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.9")

	if got := localClientIP(req); got != "203.0.113.9" {
		t.Fatalf("localClientIP = %q, want the proxy-appended %q", got, "203.0.113.9")
	}
}

func TestLocalClientIPUsesSingleForwardedEntry(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/local/claim", nil)
	req.RemoteAddr = "127.0.0.1:41234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")

	if got := localClientIP(req); got != "203.0.113.9" {
		t.Fatalf("localClientIP = %q, want %q", got, "203.0.113.9")
	}
}

// Direct access (no proxy in front) has no X-Forwarded-For at all, so the peer
// address is the only thing to key on.
func TestLocalClientIPFallsBackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/local/claim", nil)
	req.RemoteAddr = "198.51.100.7:52000"

	if got := localClientIP(req); got != "198.51.100.7" {
		t.Fatalf("localClientIP = %q, want %q", got, "198.51.100.7")
	}
}

// A caller behind one real proxy cannot escape its own bucket by prepending
// junk: every request still keys on the address Caddy observed, so the sixth
// failure is refused like any other.
func TestClaimRateLimitNotBypassableViaForwardedHeader(t *testing.T) {
	limiter := newLocalLoginLimiter()

	attempt := func(spoofed string) bool {
		req := httptest.NewRequest(http.MethodPost, "/auth/local/claim", nil)
		req.RemoteAddr = "127.0.0.1:41234"
		req.Header.Set("X-Forwarded-For", spoofed+", 203.0.113.9")
		key := localClientIP(req) + "|claim"
		if !limiter.Allow(key) {
			return false
		}
		limiter.Failure(key)
		return true
	}

	for i := range 5 {
		if !attempt(fmt.Sprintf("10.0.0.%d", i)) {
			t.Fatalf("attempt %d refused before the limit was reached", i+1)
		}
	}
	if attempt("10.0.0.99") {
		t.Fatal("a rotated X-Forwarded-For prefix bypassed the claim rate limit")
	}
}
