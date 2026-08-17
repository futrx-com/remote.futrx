package httptransport

import (
	"net"
	"net/http"
	"strings"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
)

const maxUserAgentBytes = 512

// AuditCallerFromRequest reads the network half of an audit caller — client IP
// and user agent — off an inbound request. The actor half is filled in by
// whatever resolved the session.
func AuditCallerFromRequest(r *http.Request) serviceaudit.Caller {
	if r == nil {
		return serviceaudit.Caller{}
	}
	return serviceaudit.Caller{
		IP:        ClientIP(r),
		UserAgent: truncateUserAgent(r.Header.Get("User-Agent")),
	}
}

// WithAuditCaller returns r carrying caller in its context, unless an outer
// layer already resolved one. Handlers and sockets use it so an action taken
// deep in the service layer still names the person who asked for it.
func WithAuditCaller(r *http.Request, caller serviceaudit.Caller) *http.Request {
	ctx := serviceaudit.EnsureCaller(r.Context(), caller)
	if ctx == r.Context() {
		return r
	}
	return r.WithContext(ctx)
}

// ClientIP prefers the left-most X-Forwarded-For entry: only Caddy on loopback
// can reach this backend, so that header is trustworthy here.
func ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if first := strings.TrimSpace(strings.Split(forwarded, ",")[0]); first != "" {
			return first
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func truncateUserAgent(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxUserAgentBytes {
		return value
	}
	return value[:maxUserAgentBytes]
}
