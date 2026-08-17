package httphandlers

import (
	"context"
	"errors"
	"net/http"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicenotify "github.com/futrx-com/remote.futrx.com/internal/service/notify"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// NotificationsService is the HTTP layer's narrow view of the notification
// service. Only masked configuration ever crosses this boundary.
type NotificationsService interface {
	PublicConfig() servicenotify.PublicConfig
	Save(ctx context.Context, input servicenotify.UpdateInput) (servicenotify.PublicConfig, error)
	Test(ctx context.Context) []servicenotify.SinkResult
}

// notificationsCaller resolves the authenticated principal. It exists so the
// admin gate can be exercised without building a full auth service.
type notificationsCaller interface {
	EmailAndAdmin(ctx context.Context, r *http.Request) (string, bool, error)
}

type NotificationsHandler struct {
	notifications NotificationsService
	caller        notificationsCaller
}

func NewNotificationsHandler(
	notifications NotificationsService,
	auth *serviceauth.Service,
) *NotificationsHandler {
	return &NotificationsHandler{
		notifications: notifications,
		caller:        httptransport.NewPrincipalResolver(auth),
	}
}

func (h *NotificationsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/notifications", h.handleSettings)
	mux.HandleFunc("/api/admin/notifications/test", h.handleTest)
}

func (h *NotificationsHandler) handleSettings(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		httptransport.SendJSON(w, http.StatusOK, h.notifications.PublicConfig())
	case http.MethodPut:
		var input servicenotify.UpdateInput
		if err := readJSONBody(r, &input); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid request")
			return
		}
		config, err := h.notifications.Save(r.Context(), input)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, servicenotify.ErrInvalidConfig) {
				status = http.StatusBadRequest
			}
			httptransport.SendErr(w, status, err.Error())
			return
		}
		httptransport.SendJSON(w, http.StatusOK, config)
	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *NotificationsHandler) handleTest(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	results := h.notifications.Test(r.Context())
	if results == nil {
		results = []servicenotify.SinkResult{}
	}
	httptransport.SendJSON(w, http.StatusOK, map[string]any{"results": results})
}

// authorize writes the failure response itself and reports whether the caller
// may proceed.
func (h *NotificationsHandler) authorize(w http.ResponseWriter, r *http.Request) bool {
	if h.notifications == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "notifications are unavailable")
		return false
	}
	email, isAdmin, err := h.caller.EmailAndAdmin(r.Context(), r)
	if err != nil || email == "" {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	if !isAdmin {
		httptransport.SendErr(w, http.StatusForbidden, "admin only")
		return false
	}
	return true
}
