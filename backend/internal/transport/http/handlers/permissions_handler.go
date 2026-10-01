package httphandlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// PermissionsHandler only decodes transport data. Every query and mutation is
// authorized by the existing RBAC service, including delegated management.
type PermissionsHandler struct{ service *rbac.Service }

func NewPermissionsHandler(service *rbac.Service) *PermissionsHandler {
	return &PermissionsHandler{service: service}
}
func (h *PermissionsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/permissions", h.handle)
	mux.HandleFunc("/api/permissions/", h.handle)
}

func (h *PermissionsHandler) handle(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		httptransport.SendErr(w, 503, "permissions unavailable")
		return
	}
	// Authorization is repeated by mutations under the policy lock. Checking
	// before decoding also prevents non-managers from probing input validation.
	path := strings.TrimPrefix(r.URL.Path, "/api/permissions")
	if path == "/effective" && r.Method == http.MethodGet {
		scope := rbac.PlatformScope()
		if id := r.URL.Query().Get("projectId"); id != "" {
			scope = rbac.ProjectScope(id)
		}
		decisions, err := h.service.Effective(r.Context(), scope)
		if err != nil {
			sendPermissionError(w, err)
			return
		}
		httptransport.SendJSON(w, 200, decisions)
		return
	}
	state, definitions, err := h.service.Policy(r.Context())
	if err != nil {
		sendPermissionError(w, err)
		return
	}
	if path == "" && r.Method == http.MethodGet {
		httptransport.SendJSON(w, 200, map[string]any{"definitions": definitions, "assignments": state.Assignments, "roles": state.Roles, "bindings": state.Bindings})
		return
	}
	var result any = map[string]bool{"ok": true}
	switch {
	case path == "/assignments" && r.Method == http.MethodPut:
		var in rbac.AssignmentInput
		if !decodePermissionInput(w, r, &in) {
			return
		}
		result, err = h.service.SetAssignment(r.Context(), in)
	case path == "/assignments" && r.Method == http.MethodDelete:
		var in rbac.AssignmentTarget
		if !decodePermissionInput(w, r, &in) {
			return
		}
		err = h.service.RemoveAssignment(r.Context(), in)
	case path == "/bindings" && (r.Method == http.MethodPut || r.Method == http.MethodDelete):
		var in rbac.BindingInput
		if !decodePermissionInput(w, r, &in) {
			return
		}
		if r.Method == http.MethodPut {
			result, err = h.service.BindRole(r.Context(), in)
		} else {
			err = h.service.UnbindRole(r.Context(), in)
		}
	case path == "/roles" && r.Method == http.MethodPost:
		var in rbac.RoleInput
		if !decodePermissionInput(w, r, &in) {
			return
		}
		result, err = h.service.CreateRole(r.Context(), in)
	case strings.HasPrefix(path, "/roles/") && r.Method == http.MethodPut:
		var in rbac.RoleInput
		if !decodePermissionInput(w, r, &in) {
			return
		}
		result, err = h.service.UpdateRole(r.Context(), strings.TrimPrefix(path, "/roles/"), in)
	case strings.HasPrefix(path, "/roles/") && r.Method == http.MethodDelete:
		// A bound role must be explicitly unbound first; no implicit bulk removal.
		err = h.service.DeleteRole(r.Context(), strings.TrimPrefix(path, "/roles/"), rbac.DeleteRoleOptions{})
	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		sendPermissionError(w, err)
		return
	}
	httptransport.SendJSON(w, 200, result)
}

func decodePermissionInput(w http.ResponseWriter, r *http.Request, out any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		httptransport.SendErr(w, 400, "invalid permission input")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		httptransport.SendErr(w, 400, "expected one JSON object")
		return false
	}
	return true
}
func sendPermissionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, rbac.ErrDenied), errors.Is(err, rbac.ErrActorRequired):
		httptransport.SendErr(w, 403, "permission denied")
	case errors.Is(err, rbac.ErrRoleNotFound):
		httptransport.SendErr(w, 404, "role not found")
	case errors.Is(err, rbac.ErrRoleInUse):
		httptransport.SendErr(w, 409, "remove role bindings before deleting this role")
	case errors.Is(err, rbac.ErrInvalidScope), errors.Is(err, rbac.ErrUnknownPermission), errors.Is(err, rbac.ErrInvalidEffect), errors.Is(err, rbac.ErrInvalidRole), errors.Is(err, rbac.ErrUserNotRegistered):
		httptransport.SendErr(w, 400, err.Error())
	default:
		httptransport.SendErr(w, 500, "permission operation failed")
	}
}
