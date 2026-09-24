package httphandlers

import (
	"context"
	"io"
	"net/http"
	"strings"

	serviceapplications "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// backendAction is the path segment that routes a request to an instance's Go
// backend instead of to the applications API.
const backendAction = "backend"

// maxBackendRequestBody bounds what a browser may hand a backend. A backend is a
// child process on the host, so the size of what reaches it is the server's
// decision, not the caller's.
const maxBackendRequestBody = 1 << 20 // 1 MiB

// isBackendPath reports whether an instance action addresses its backend, and
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

// serveBackend answers the two things an instance's backend route does: GET on
// the bare prefix describes the backend, and anything deeper is forwarded to it.
//
// Authorization has already established that the caller may reach this
// instance — project membership for a project install, being signed in for a
// global one. What the backend will do for them is the backend's decision, made
// against the caller the service stamps onto the request; the application's `access`
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
	h.serveBackendAs(w, r, id, path, caller, nil)
}

// HandleChatBackend serves the backend subresource of an already-authorized
// chat. ChatHandler resolves membership before delegating here; this method
// turns that result into context the application may trust.
func (h *ApplicationsHandler) HandleChatBackend(
	w http.ResponseWriter,
	r *http.Request,
	meta servicechat.Meta,
	caller applications.Caller,
	resource string,
) {
	if h.apps == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "applications unavailable")
		return
	}
	id, action := parseApplicationResource(resource)
	path, ok := isBackendPath(action)
	if id == "" || !ok {
		httptransport.SendErr(w, http.StatusNotFound, "not found")
		return
	}
	chat, err := h.trustedChatContext(r.Context(), meta)
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "chat workspace is unavailable")
		return
	}
	h.serveBackendAs(w, r, id, path, caller, &chat)
}

func (h *ApplicationsHandler) trustedChatContext(
	ctx context.Context,
	meta servicechat.Meta,
) (applications.ChatContext, error) {
	if h.chatWorkspaces == nil {
		return applications.ChatContext{}, servicechat.ErrWorkspaceUnavailable
	}
	root, err := h.chatWorkspaces.TrustedWorkspaceRoot(ctx, meta)
	if err != nil {
		return applications.ChatContext{}, err
	}
	return applications.ChatContext{
		ID:            string(meta.ID),
		ProjectID:     string(meta.ProjectID),
		WorkspaceRoot: root,
	}, nil
}

func (h *ApplicationsHandler) serveBackendAs(
	w http.ResponseWriter,
	r *http.Request,
	id string,
	path string,
	caller applications.Caller,
	chat *applications.ChatContext,
) {

	if path == "" {
		if r.Method != http.MethodGet {
			httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var descriptor serviceapplications.BackendDescriptor
		var err error
		if chat == nil {
			descriptor, err = h.apps.DescribeBackend(r.Context(), id, caller)
		} else {
			descriptor, err = h.apps.DescribeBackendForChat(r.Context(), id, caller, *chat)
		}
		if err != nil {
			sendAppError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, descriptor)
		return
	}

	body, ok := readBackendRequestBody(w, r)
	if !ok {
		return
	}
	request := applications.Request{
		Method:  r.Method,
		Path:    path,
		Query:   r.URL.Query(),
		Headers: forwardableHeaders(r.Header),
		Body:    body,
	}
	var (
		response applications.Response
		err      error
	)
	if chat == nil {
		response, err = h.apps.CallBackend(r.Context(), id, request, caller)
	} else {
		response, err = h.apps.CallBackendForChat(r.Context(), id, request, caller, *chat)
	}
	if err != nil {
		sendAppError(w, err)
		return
	}
	writeBackendResponse(w, r, response)
}

// readBackendRequestBody reads one byte beyond the public limit so oversized
// input is rejected instead of silently forwarding a valid-looking truncated
// prefix to the application.
func readBackendRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBackendRequestBody+1))
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "unreadable request body")
		return nil, false
	}
	if len(body) > maxBackendRequestBody {
		httptransport.SendErr(w, http.StatusRequestEntityTooLarge, "request body exceeds 1 MiB limit")
		return nil, false
	}
	return body, true
}

// backendCaller resolves the signed-in user a backend will see. A backend is
// told who is asking so it can authorize them itself, so an unidentifiable
// caller is refused here rather than reaching one anonymously.
func (h *ApplicationsHandler) backendCaller(w http.ResponseWriter, r *http.Request) (applications.Caller, bool) {
	if h.auth == nil {
		return applications.Caller{}, true
	}
	email, err := callerEmailFromRequest(r, h.auth)
	if err != nil || email == "" {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return applications.Caller{}, false
	}
	isAdmin, _ := h.auth.IsAdmin(r.Context(), email)
	return applications.Caller{Email: email, IsAdmin: isAdmin}, true
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

var streamManagedHeaders = map[string]bool{
	"accept-ranges": true,
	"content-range": true,
	"last-modified": true,
}

// forwardableHeaders copies the request headers a backend may usefully read.
// Credentials are withheld: the backend is told who the caller is through
// Request.Caller, and giving it their session cookie as well would hand every
// backend the ability to act as them against the rest of the API.
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

// writeBackendResponse turns a backend's answer back into an HTTP response.
// Set-Cookie is dropped because a backend's response is same-origin with the
// SPA and must not be able to write the session; the content type is pinned
// with nosniff for the same reason ui/ assets are.
func writeBackendResponse(w http.ResponseWriter, request *http.Request, response applications.Response) {
	content, _, modTime, streaming := response.ResponseStream()
	for name, values := range response.Headers {
		lower := strings.ToLower(name)
		if hopByHopHeaders[lower] || lower == "set-cookie" || lower == "content-length" ||
			(streaming && streamManagedHeaders[lower]) {
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

	if streaming {
		// The request context outlives the manifest's backend-call timeout: that
		// timeout only bounds opening a response. Once open, a stream is closed
		// by completion, caller cancellation, or connection failure.
		stopClose := context.AfterFunc(request.Context(), func() { _ = content.Close() })
		defer func() {
			stopClose()
			_ = content.Close()
		}()
		http.ServeContent(w, request, "", modTime, content)
		return
	}

	status := response.Status
	if status < 100 || status > 599 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(response.Body)
}
