package wstransport

import (
	"net/http"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// auditCaller builds the audit identity for a WebSocket upgrade. Sockets
// resolve the caller themselves (the auth middleware has already run, but a
// long-lived socket needs a context that outlives the request), so this keeps
// the IP/user-agent extraction identical to the HTTP path.
func auditCaller(r *http.Request, email string, isAdmin bool) serviceaudit.Caller {
	caller := httptransport.AuditCallerFromRequest(r)
	caller.Actor = serviceaudit.Actor{Email: email, IsAdmin: isAdmin}
	if resolved, ok := serviceaudit.CallerFrom(r.Context()); ok {
		caller.Actor.Sub = resolved.Actor.Sub
		if caller.Actor.Email == "" {
			caller.Actor = resolved.Actor
		}
	}
	return caller
}
