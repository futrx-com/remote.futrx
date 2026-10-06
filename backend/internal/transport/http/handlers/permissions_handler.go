package httphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	servicepermission "github.com/futrx-com/remote.futrx.com/internal/rbac"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

const (
	permissionsRequestLimit = 1 << 16
	permissionsRolesPath    = "/api/admin/permissions/roles"
)

// PermissionsService is the HTTP layer's narrow view of the permission
// policy. Authorization lives inside the service: reads require either
// management permission, and writes require the one that owns them.
type PermissionsService interface {
	Definitions(ctx context.Context) ([]servicepermission.Definition, error)
	Roles(ctx context.Context) ([]servicepermission.Role, error)
	Assignments(ctx context.Context) ([]servicepermission.Assignment, error)
	Bindings(ctx context.Context) ([]servicepermission.RoleBinding, error)
	SetAssignment(ctx context.Context, in servicepermission.AssignmentInput) (servicepermission.Assignment, error)
	RemoveAssignment(ctx context.Context, target servicepermission.AssignmentTarget) error
	BindRole(ctx context.Context, in servicepermission.BindingInput) (servicepermission.RoleBinding, error)
	UnbindRole(ctx context.Context, in servicepermission.BindingInput) error
	CreateRole(ctx context.Context, in servicepermission.RoleInput) (servicepermission.Role, error)
	UpdateRole(ctx context.Context, id string, in servicepermission.RoleInput) (servicepermission.Role, error)
	DeleteRole(ctx context.Context, id string, options servicepermission.DeleteRoleOptions) error
}

type PermissionsHandler struct {
	permissions PermissionsService
}

func NewPermissionsHandler(permissions PermissionsService) *PermissionsHandler {
	return &PermissionsHandler{permissions: permissions}
}

func (h *PermissionsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/permissions/definitions", h.HandleDefinitions)
	mux.HandleFunc(permissionsRolesPath, h.HandleRoles)
	mux.HandleFunc(permissionsRolesPath+"/", h.HandleRole)
	mux.HandleFunc("/api/admin/permissions/assignments", h.HandleAssignments)
	mux.HandleFunc("/api/admin/permissions/bindings", h.HandleBindings)
}

type permissionScopeBody struct {
	Kind servicepermission.ScopeKind `json:"kind"`
	ID   string                      `json:"id,omitempty"`
}

func (s permissionScopeBody) scope() servicepermission.Scope {
	return servicepermission.Scope{Kind: s.Kind, ID: s.ID}
}

type permissionRuleBody struct {
	Permission servicepermission.Key    `json:"permission"`
	Effect     servicepermission.Effect `json:"effect"`
}

type permissionRoleBody struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Rules       []permissionRuleBody `json:"rules"`
}

func (b permissionRoleBody) input() servicepermission.RoleInput {
	rules := make([]servicepermission.RoleRule, 0, len(b.Rules))
	for _, rule := range b.Rules {
		rules = append(rules, servicepermission.RoleRule{Permission: rule.Permission, Effect: rule.Effect})
	}
	return servicepermission.RoleInput{Name: b.Name, Description: b.Description, Rules: rules}
}

type permissionAssignmentBody struct {
	UserEmail  string                   `json:"userEmail"`
	Permission servicepermission.Key    `json:"permission"`
	Effect     servicepermission.Effect `json:"effect"`
	Scope      permissionScopeBody      `json:"scope"`
}

type permissionBindingBody struct {
	UserEmail string              `json:"userEmail"`
	RoleID    string              `json:"roleId"`
	Scope     permissionScopeBody `json:"scope"`
}

func (h *PermissionsHandler) HandleDefinitions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	definitions, err := h.permissions.Definitions(r.Context())
	if err != nil {
		sendPermissionsError(w, err)
		return
	}
	if definitions == nil {
		definitions = []servicepermission.Definition{}
	}
	httptransport.SendJSON(w, http.StatusOK, map[string]any{"definitions": definitions})
}

func (h *PermissionsHandler) HandleRoles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		roles, err := h.permissions.Roles(r.Context())
		if err != nil {
			sendPermissionsError(w, err)
			return
		}
		if roles == nil {
			roles = []servicepermission.Role{}
		}
		httptransport.SendJSON(w, http.StatusOK, map[string]any{"roles": roles})
	case http.MethodPost:
		var body permissionRoleBody
		if !decodePermissionsBody(w, r, &body) {
			return
		}
		role, err := h.permissions.CreateRole(r.Context(), body.input())
		if err != nil {
			sendPermissionsError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, role)
	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleRole serves /api/admin/permissions/roles/{id}.
func (h *PermissionsHandler) HandleRole(w http.ResponseWriter, r *http.Request) {
	id, err := url.PathUnescape(strings.Trim(strings.TrimPrefix(r.URL.Path, permissionsRolesPath), "/"))
	if err != nil || id == "" || strings.Contains(id, "/") {
		httptransport.SendErr(w, http.StatusBadRequest, "missing role id")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var body permissionRoleBody
		if !decodePermissionsBody(w, r, &body) {
			return
		}
		role, err := h.permissions.UpdateRole(r.Context(), id, body.input())
		if err != nil {
			sendPermissionsError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, role)
	case http.MethodDelete:
		options := servicepermission.DeleteRoleOptions{Unbind: r.URL.Query().Get("unbind") == "true"}
		if err := h.permissions.DeleteRole(r.Context(), id, options); err != nil {
			sendPermissionsError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *PermissionsHandler) HandleAssignments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		assignments, err := h.permissions.Assignments(r.Context())
		if err != nil {
			sendPermissionsError(w, err)
			return
		}
		if assignments == nil {
			assignments = []servicepermission.Assignment{}
		}
		httptransport.SendJSON(w, http.StatusOK, map[string]any{"assignments": assignments})
	case http.MethodPost:
		var body permissionAssignmentBody
		if !decodePermissionsBody(w, r, &body) {
			return
		}
		assignment, err := h.permissions.SetAssignment(r.Context(), servicepermission.AssignmentInput{
			UserEmail:  body.UserEmail,
			Permission: body.Permission,
			Effect:     body.Effect,
			Scope:      body.Scope.scope(),
		})
		if err != nil {
			sendPermissionsError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, assignment)
	case http.MethodDelete:
		query := r.URL.Query()
		email, permission := query.Get("userEmail"), query.Get("permission")
		if email == "" || permission == "" || query.Get("scopeKind") == "" {
			httptransport.SendErr(w, http.StatusBadRequest, "userEmail, permission and scopeKind are required")
			return
		}
		err := h.permissions.RemoveAssignment(r.Context(), servicepermission.AssignmentTarget{
			UserEmail:  email,
			Permission: servicepermission.Key(permission),
			Scope:      queryPermissionScope(query),
		})
		if err != nil {
			sendPermissionsError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *PermissionsHandler) HandleBindings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		bindings, err := h.permissions.Bindings(r.Context())
		if err != nil {
			sendPermissionsError(w, err)
			return
		}
		if bindings == nil {
			bindings = []servicepermission.RoleBinding{}
		}
		httptransport.SendJSON(w, http.StatusOK, map[string]any{"bindings": bindings})
	case http.MethodPost:
		var body permissionBindingBody
		if !decodePermissionsBody(w, r, &body) {
			return
		}
		binding, err := h.permissions.BindRole(r.Context(), servicepermission.BindingInput{
			RoleID:    body.RoleID,
			UserEmail: body.UserEmail,
			Scope:     body.Scope.scope(),
		})
		if err != nil {
			sendPermissionsError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, binding)
	case http.MethodDelete:
		query := r.URL.Query()
		email, roleID := query.Get("userEmail"), query.Get("roleId")
		if email == "" || roleID == "" || query.Get("scopeKind") == "" {
			httptransport.SendErr(w, http.StatusBadRequest, "userEmail, roleId and scopeKind are required")
			return
		}
		err := h.permissions.UnbindRole(r.Context(), servicepermission.BindingInput{
			RoleID:    roleID,
			UserEmail: email,
			Scope:     queryPermissionScope(query),
		})
		if err != nil {
			sendPermissionsError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func queryPermissionScope(query url.Values) servicepermission.Scope {
	return servicepermission.Scope{
		Kind: servicepermission.ScopeKind(query.Get("scopeKind")),
		ID:   query.Get("scopeId"),
	}
}

func decodePermissionsBody(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, permissionsRequestLimit)).Decode(into); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

func sendPermissionsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, servicepermission.ErrActorRequired):
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
	case errors.Is(err, servicepermission.ErrDenied):
		httptransport.SendErr(w, http.StatusForbidden, err.Error())
	case errors.Is(err, servicepermission.ErrRoleNotFound):
		httptransport.SendErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, servicepermission.ErrRoleInUse):
		httptransport.SendErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, servicepermission.ErrInvalidRole),
		errors.Is(err, servicepermission.ErrInvalidScope),
		errors.Is(err, servicepermission.ErrInvalidEffect),
		errors.Is(err, servicepermission.ErrUnknownPermission),
		errors.Is(err, servicepermission.ErrUserNotRegistered):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
