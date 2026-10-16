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
	serviceinvitation "github.com/futrx-com/remote.futrx.com/internal/service/invitation"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// UsersHandler exposes admin-only CRUD for the users directory. Every route
// re-checks `IsAdmin` so a registered non-admin who fishes for the URL hits
// a clean 403 (the broader API gate only checks `IsRegistered`).
type UsersHandler struct {
	users       *serviceuser.Service
	invitations userInvitations
	auth        *serviceauth.Service
}

// userInvitations is what the handler needs of the invitation service.
type userInvitations interface {
	Invite(ctx context.Context, req serviceinvitation.InviteRequest) (serviceinvitation.Result, error)
	Reset(ctx context.Context, req serviceinvitation.ResetRequest) (serviceinvitation.Result, error)
	Remove(ctx context.Context, email string) error
}

func NewUsersHandler(users *serviceuser.Service, invitations userInvitations, auth *serviceauth.Service) *UsersHandler {
	return &UsersHandler{users: users, invitations: invitations, auth: auth}
}

// userListItem is a directory entry joined with how the account signs in. The
// join is presentation only; the policy lives in the auth service.
type userListItem struct {
	serviceuser.User
	PasswordSignIn        bool `json:"passwordSignIn"`
	PasswordChangePending bool `json:"passwordChangePending"`
}

// inviteResult is the response of a password invite or a reset. It carries
// the temporary password, so it is never cached.
type inviteResult struct {
	User              serviceuser.User `json:"user"`
	Method            string           `json:"method"`
	Delivery          string           `json:"delivery"`
	DeliveryError     string           `json:"deliveryError,omitempty"`
	TemporaryPassword string           `json:"temporaryPassword"`
}

func sendInviteResult(w http.ResponseWriter, status int, result serviceinvitation.Result) {
	w.Header().Set("Cache-Control", "no-store")
	httptransport.SendJSON(w, status, inviteResult{
		User:              result.User,
		Method:            string(result.Method),
		Delivery:          string(result.Delivery),
		DeliveryError:     result.DeliveryError,
		TemporaryPassword: result.TemporaryPassword,
	})
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
		states := h.auth.PasswordStates(r.Context())
		items := make([]userListItem, 0, len(list))
		for _, u := range list {
			state, invited := states[strings.ToLower(strings.TrimSpace(u.Email))]
			items = append(items, userListItem{
				User:                  u,
				PasswordSignIn:        invited || h.auth.IsLocalAdmin(u.Email),
				PasswordChangePending: invited && state.ChangePending,
			})
		}
		httptransport.SendJSON(w, http.StatusOK, items)

	case http.MethodPost:
		h.invite(w, r, callerEmail)

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// invite creates an account. A request without `method` means Google, and for
// that path the "Google must be configured" refusal comes before the body is
// read, exactly as it did before passwords existed.
func (h *UsersHandler) invite(w http.ResponseWriter, r *http.Request, callerEmail string) {
	var body struct {
		Email     string `json:"email"`
		Role      string `json:"role"`
		Method    string `json:"method"`
		SendEmail *bool  `json:"sendEmail"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)
	method := strings.ToLower(strings.TrimSpace(body.Method))
	if decodeErr != nil || method == "" || method == string(serviceinvitation.MethodGoogle) {
		if err := h.auth.RequireGoogleSignIn(); err != nil {
			httptransport.SendErr(w, http.StatusConflict, err.Error())
			return
		}
	}
	if decodeErr != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	role := serviceuser.NormalizeRole(body.Role)
	if role == "" {
		httptransport.SendErr(w, http.StatusBadRequest, "role must be 'admin' or 'member'")
		return
	}
	result, err := h.invitations.Invite(r.Context(), serviceinvitation.InviteRequest{
		Email:     body.Email,
		Role:      role,
		Method:    serviceinvitation.Method(method),
		SendEmail: body.SendEmail == nil || *body.SendEmail,
		InvitedBy: callerEmail,
	})
	if err != nil {
		sendUsersError(w, err)
		return
	}
	if result.Method == serviceinvitation.MethodPassword {
		sendInviteResult(w, http.StatusCreated, result)
		return
	}
	httptransport.SendJSON(w, http.StatusCreated, result.User)
}

// reset issues a new temporary password for an existing user.
func (h *UsersHandler) reset(w http.ResponseWriter, r *http.Request, callerEmail, email string) {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		SendEmail *bool `json:"sendEmail"`
	}
	// The body is optional: an empty one means the default (send the email).
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	result, err := h.invitations.Reset(r.Context(), serviceinvitation.ResetRequest{
		Email:     email,
		Actor:     callerEmail,
		SendEmail: body.SendEmail == nil || *body.SendEmail,
	})
	if err != nil {
		sendUsersError(w, err)
		return
	}
	sendInviteResult(w, http.StatusOK, result)
}

func (h *UsersHandler) HandleResource(w http.ResponseWriter, r *http.Request) {
	callerEmail, ok := h.requireAdmin(w, r)
	if !ok {
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

	if len(parts) == 2 && parts[1] == "temporary-password" {
		h.reset(w, r, callerEmail, email)
		return
	}
	if len(parts) == 2 && parts[1] != "" && parts[1] != "role" {
		httptransport.SendErr(w, http.StatusNotFound, "not found")
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
		if role == "" {
			httptransport.SendErr(w, http.StatusBadRequest, "role must be 'admin' or 'member'")
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
		if err := h.invitations.Remove(r.Context(), email); err != nil {
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
	if h.users == nil || h.auth == nil || h.invitations == nil {
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
	case errors.Is(err, serviceuser.ErrInvalidEmail), errors.Is(err, serviceuser.ErrInvalidRole),
		errors.Is(err, serviceinvitation.ErrInvalidMethod):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, serviceuser.ErrUserNotFound):
		httptransport.SendErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, serviceuser.ErrUserExists),
		errors.Is(err, serviceuser.ErrCannotDemoteLastAdmin),
		errors.Is(err, serviceuser.ErrCannotRemoveLastAdmin),
		errors.Is(err, serviceauth.ErrGoogleSignInRequired),
		errors.Is(err, serviceinvitation.ErrResetSelf),
		errors.Is(err, serviceinvitation.ErrResetLocalAdmin):
		httptransport.SendErr(w, http.StatusConflict, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
