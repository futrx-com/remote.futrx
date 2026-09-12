package httphandlers

import (
	"context"
	"errors"
	"net/http"

	serviceresources "github.com/futrx-com/remote.futrx.com/internal/service/resources"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// ResourcePolicyService is the admin-facing half of the fleet resource policy.
type ResourcePolicyService interface {
	Get(ctx context.Context) serviceresources.View
	Update(ctx context.Context, in serviceresources.Settings, actor string) (serviceresources.View, error)
}

// CallerResolver reports the authenticated caller and whether they are an
// admin. Declared here rather than taking the concrete auth service so the
// admin gate is testable without a session codec.
type CallerResolver interface {
	EmailAndAdmin(ctx context.Context, r *http.Request) (string, bool, error)
}

// AdminResourcesHandler exposes the operator-editable fleet resource policy:
// the defaults every container inherits, the host reserve, the per-project
// override ceiling, the running-container cap, and the storage pool's
// disk-quota capability.
type AdminResourcesHandler struct {
	resources ResourcePolicyService
	caller    CallerResolver
}

func NewAdminResourcesHandler(resources ResourcePolicyService, caller CallerResolver) *AdminResourcesHandler {
	return &AdminResourcesHandler{resources: resources, caller: caller}
}

func (h *AdminResourcesHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/resources", h.handleResources)
}

func (h *AdminResourcesHandler) handleResources(w http.ResponseWriter, r *http.Request) {
	if h.resources == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "resource policy is unavailable")
		return
	}
	if !h.requireAdmin(w, r) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		httptransport.SendJSON(w, http.StatusOK, h.resources.Get(r.Context()))

	case http.MethodPut:
		var body serviceresources.Settings
		if err := readJSONBody(r, &body); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		email, _, _ := h.caller.EmailAndAdmin(r.Context(), r)
		view, err := h.resources.Update(r.Context(), body, email)
		switch {
		case errors.Is(err, serviceresources.ErrInvalidSettings):
			httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		case err != nil:
			// The policy is persisted; only the live LXD convergence failed.
			// Report it so the operator knows a container restart may be
			// needed, without pretending the change was lost.
			httptransport.SendErr(w, http.StatusBadGateway, err.Error())
		default:
			httptransport.SendJSON(w, http.StatusOK, view)
		}

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *AdminResourcesHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if h.caller == nil {
		httptransport.SendErr(w, http.StatusForbidden, "admin only")
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
