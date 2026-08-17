package httptransport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
)

func TestClientIPPrefersTheForwardedChain(t *testing.T) {
	tests := []struct {
		name      string
		forwarded string
		remote    string
		want      string
	}{
		{name: "forwarded chain", forwarded: "203.0.113.7, 10.0.0.2", remote: "127.0.0.1:5000", want: "203.0.113.7"},
		{name: "empty forwarded header", forwarded: "", remote: "127.0.0.1:5000", want: "127.0.0.1"},
		{name: "remote addr without a port", forwarded: "", remote: "127.0.0.1", want: "127.0.0.1"},
		{name: "blank forwarded entry", forwarded: " , 10.0.0.2", remote: "127.0.0.1:5000", want: "127.0.0.1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
			req.RemoteAddr = tc.remote
			if tc.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tc.forwarded)
			}
			if got := ClientIP(req); got != tc.want {
				t.Fatalf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAuditCallerFromRequestTruncatesLongUserAgents(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Header.Set("User-Agent", strings.Repeat("x", maxUserAgentBytes+50))

	caller := AuditCallerFromRequest(req)
	if len(caller.UserAgent) != maxUserAgentBytes {
		t.Fatalf("user agent length = %d, want %d", len(caller.UserAgent), maxUserAgentBytes)
	}
}

func TestWithAuditCallerKeepsAnExistingCaller(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req = req.WithContext(serviceaudit.WithCaller(
		req.Context(),
		serviceaudit.Caller{Actor: serviceaudit.Actor{Email: "first@example.com"}},
	))

	updated := WithAuditCaller(req, serviceaudit.Caller{Actor: serviceaudit.Actor{Email: "second@example.com"}})
	if updated != req {
		t.Fatal("the request was rewrapped even though a caller was already resolved")
	}
	caller, _ := serviceaudit.CallerFrom(updated.Context())
	if caller.Actor.Email != "first@example.com" {
		t.Fatalf("actor = %q, want the original", caller.Actor.Email)
	}
}
