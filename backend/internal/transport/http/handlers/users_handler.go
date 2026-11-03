package httphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// UsersHandler exposes admin-only CRUD for the users directory. Every route
// re-checks `IsAdmin` so a registered non-admin who fishes for the URL hits
// a clean 403 (the broader API gate only checks `IsRegistered`).
type UsersHandler struct {
	users *serviceuser.Service
	auth  *serviceauth.Service
}

func NewUsersHandler(users *serviceuser.Service, auth *serviceauth.Service) *UsersHandler {
	return &UsersHandler{users: users, auth: auth}
}

func (h *UsersHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/users", h.HandleCollection)
	mux.HandleFunc("/api/admin/users/", h.HandleResource)
}

func (h *UsersHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	callerEmail, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		list, err := h.users.List(r.Context())
		if err != nil {
			sendUsersError(w, err)
			return
		}
		if list == nil {
			list = []serviceuser.User{}
		}
		httptransport.SendJSON(w, http.StatusOK, list)

	case http.MethodPost:
		if !h.auth.GoogleOAuthEnabled() {
			httptransport.SendErr(w, http.StatusConflict, "configure Google sign-in before adding users")
			return
		}
		var body struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		role := serviceuser.NormalizeRole(body.Role)
		if !serviceuser.ValidRole(role) {
			httptransport.SendErr(w, http.StatusBadRequest, "role must be 'admin' or empty")
			return
		}
		u, err := h.users.Add(r.Context(), body.Email, role, callerEmail)
		if err != nil {
			sendUsersError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusCreated, u)

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *UsersHandler) HandleResource(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/users/")
	parts := strings.SplitN(rest, "/", 2)
	emailRaw := parts[0]
	if emailRaw == "" {
		httptransport.SendErr(w, http.StatusBadRequest, "missing email")
		return
	}
	email, err := url.PathUnescape(emailRaw)
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid email path")
		return
	}

	if len(parts) == 2 && parts[1] == "role" {
		if r.Method != http.MethodPut {
			httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		role := serviceuser.NormalizeRole(body.Role)
		if !serviceuser.ValidRole(role) {
			httptransport.SendErr(w, http.StatusBadRequest, "role must be 'admin' or empty")
			return
		}
		if h.auth.IsLocalAdmin(email) && role != serviceuser.RoleAdmin {
			httptransport.SendErr(w, http.StatusConflict, "the local administrator cannot be demoted")
			return
		}
		u, err := h.users.SetRole(r.Context(), email, role)
		if err != nil {
			sendUsersError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, u)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if h.auth.IsLocalAdmin(email) {
			httptransport.SendErr(w, http.StatusConflict, "the local administrator cannot be removed")
			return
		}
		if err := h.users.Remove(r.Context(), email); err != nil {
			sendUsersError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// requireAdmin verifies the caller's session belongs to a user with the
// admin role. Returns the caller's normalized email + true on pass, "" +
// false on fail (in which case it has already written a response).
func (h *UsersHandler) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	if h.users == nil || h.auth == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "users service unavailable")
		return "", false
	}
	email, err := callerEmailFromRequest(r, h.auth)
	if err != nil || email == "" {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return "", false
	}
	ok, _ := h.auth.IsAdmin(r.Context(), email)
	if !ok {
		httptransport.SendErr(w, http.StatusForbidden, "admin only")
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(email)), true
}

// callerEmailFromRequest reads the session cookie and returns the email it
// authenticates. Useful from any handler that needs to know "who is asking
// right now" without baking auth into its parameter list.
func callerEmailFromRequest(r *http.Request, auth *serviceauth.Service) (string, error) {
	return httptransport.NewPrincipalResolver(auth).Email(r)
}

// callerStateFromRequest returns the caller email and their isAdmin flag.
// Used by per-project handlers that need both bits and don't want to call
// the auth service twice.
func callerStateFromRequest(ctx context.Context, r *http.Request, auth *serviceauth.Service) (string, bool, error) {
	return httptransport.NewPrincipalResolver(auth).EmailAndAdmin(ctx, r)
}

func sendUsersError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, serviceuser.ErrInvalidEmail), errors.Is(err, serviceuser.ErrInvalidRole):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, serviceuser.ErrUserNotFound):
		httptransport.SendErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, serviceuser.ErrUserExists),
		errors.Is(err, serviceuser.ErrCannotDemoteLastAdmin),
		errors.Is(err, serviceuser.ErrCannotRemoveLastAdmin):
		httptransport.SendErr(w, http.StatusConflict, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
