package httphandlers

import (
	"io"
	"net/http"
	"strings"

	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// backendAction is the path segment that routes a request to an instance's Go
// plugin instead of to the applications API.
const backendAction = "backend"

// maxBackendRequestBody bounds what a browser may hand a plugin. A plugin is a
// child process on the host, so the size of what reaches it is the server's
// decision, not the caller's.
const maxBackendRequestBody = 1 << 20 // 1 MiB

// isBackendPath reports whether an instance action addresses its plugin, and
// returns the path to forward, relative to the /backend/ prefix.
func isBackendPath(action string) (string, bool) {
	if action == backendAction {
		return "", true
	}
	if rest, ok := strings.CutPrefix(action, backendAction+"/"); ok {
		return rest, true
	}
	return "", false
}

// serveBackend answers the two things an instance's plugin route does: GET on
// the bare prefix describes the plugin, and anything deeper is forwarded to it.
//
// Authorization has already established that the caller may reach this
// instance — project membership for a project install, being signed in for a
// global one. What the plugin will do for them is the plugin's decision, made
// against the caller the service stamps onto the request; the image's `access`
// level is the only part of that the platform enforces itself.
func (h *ApplicationsHandler) serveBackend(w http.ResponseWriter, r *http.Request, id, path string) {
	if h.apps == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "applications unavailable")
		return
	}
	caller, ok := h.backendCaller(w, r)
	if !ok {
		return
	}

	if path == "" {
		if r.Method != http.MethodGet {
			httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		descriptor, err := h.apps.DescribeBackend(r.Context(), id, caller)
		if err != nil {
			sendAppError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, descriptor)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBackendRequestBody))
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "unreadable request body")
		return
	}
	response, err := h.apps.CallBackend(r.Context(), id, appplugin.Request{
		Method:  r.Method,
		Path:    path,
		Query:   r.URL.Query(),
		Headers: forwardableHeaders(r.Header),
		Body:    body,
	}, caller)
	if err != nil {
		sendAppError(w, err)
		return
	}
	writeBackendResponse(w, response)
}

// backendCaller resolves the signed-in user a plugin will see. A plugin is
// told who is asking so it can authorize them itself, so an unidentifiable
// caller is refused here rather than reaching one anonymously.
func (h *ApplicationsHandler) backendCaller(w http.ResponseWriter, r *http.Request) (appplugin.Caller, bool) {
	if h.auth == nil {
		return appplugin.Caller{}, true
	}
	email, err := callerEmailFromRequest(r, h.auth)
	if err != nil || email == "" {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return appplugin.Caller{}, false
	}
	isAdmin, _ := h.auth.IsAdmin(r.Context(), email)
	return appplugin.Caller{Email: email, IsAdmin: isAdmin}, true
}

// hopByHopHeaders belong to one connection and must not be copied across a
// forwarding boundary in either direction.
var hopByHopHeaders = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
}

// forwardableHeaders copies the request headers a plugin may usefully read.
// Credentials are withheld: the plugin is told who the caller is through
// Request.Caller, and giving it their session cookie as well would hand every
// plugin the ability to act as them against the rest of the API.
func forwardableHeaders(header http.Header) map[string][]string {
	forwarded := make(map[string][]string, len(header))
	for name, values := range header {
		lower := strings.ToLower(name)
		if hopByHopHeaders[lower] || lower == "cookie" || lower == "authorization" {
			continue
		}
		forwarded[name] = append([]string(nil), values...)
	}
	return forwarded
}

// writeBackendResponse turns a plugin's answer back into an HTTP response.
// Set-Cookie is dropped because a plugin's response is same-origin with the
// SPA and must not be able to write the session; the content type is pinned
// with nosniff for the same reason ui/ assets are.
func writeBackendResponse(w http.ResponseWriter, response appplugin.Response) {
	for name, values := range response.Headers {
		lower := strings.ToLower(name)
		if hopByHopHeaders[lower] || lower == "set-cookie" || lower == "content-length" {
			continue
		}
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")

	status := response.Status
	if status < 100 || status > 599 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(response.Body)
}
