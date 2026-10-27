package httphandlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentcapability "github.com/futrx-com/remote.futrx.com/internal/service/agent/capability"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

type AgentCapabilitiesService interface {
	List(context.Context, agentcapability.ListQuery) ([]agent.Capabilities, error)
}

type AgentCapabilitiesHandler struct {
	catalog AgentCapabilitiesService
}

func NewAgentCapabilitiesHandler(catalog AgentCapabilitiesService) *AgentCapabilitiesHandler {
	return &AgentCapabilitiesHandler{catalog: catalog}
}

func (h *AgentCapabilitiesHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/agent-capabilities", h.HandleCollection)
}

func (h *AgentCapabilitiesHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.catalog == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "agent capabilities unavailable")
		return
	}
	items, err := h.catalog.List(r.Context(), agentcapability.ListQuery{
		ProjectID:     serviceproject.ID(strings.TrimSpace(r.URL.Query().Get("projectId"))),
		SessionCookie: httptransport.SessionCookieValue(r),
		Refresh:       r.URL.Query().Get("refresh") == "1",
	})
	if err != nil {
		sendAgentCapabilitiesError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, map[string][]agent.Capabilities{"providers": items})
}

func sendAgentCapabilitiesError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, agentcapability.ErrProjectLookupUnavailable):
		httptransport.SendErr(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, agentcapability.ErrProjectNotFound):
		httptransport.SendErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, agentcapability.ErrAuthenticationRequired):
		httptransport.SendErr(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, agentcapability.ErrProjectAccessDenied):
		httptransport.SendErr(w, http.StatusForbidden, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
