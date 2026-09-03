package httphandlers

import (
	"net/http"

	agentquota "github.com/futrx-com/remote.futrx.com/internal/service/agent/quota"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// AgentQuotaService reports the last subscription window each agent mentioned.
type AgentQuotaService interface {
	View() []agentquota.AgentQuota
}

// AgentQuotaHandler serves the home screen's plan card.
type AgentQuotaHandler struct {
	quota AgentQuotaService
	auth  *serviceauth.Service
}

func NewAgentQuotaHandler(quota AgentQuotaService, auth *serviceauth.Service) *AgentQuotaHandler {
	return &AgentQuotaHandler{quota: quota, auth: auth}
}

func (h *AgentQuotaHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/agent-quota", h.handle)
}

// handle answers any signed-in user.
//
// An empty list is a real answer, not an error: readings only arrive while an
// agent runs, so a platform nobody has used yet genuinely knows nothing. The
// browser is expected to say "no reading yet" rather than draw an empty gauge.
func (h *AgentQuotaHandler) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h == nil || h.quota == nil {
		httptransport.SendJSON(w, http.StatusOK, map[string]any{"agents": []agentquota.AgentQuota{}})
		return
	}
	email, _, err := httptransport.NewPrincipalResolver(h.auth).EmailAndAdmin(r.Context(), r)
	if err != nil || email == "" {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	agents := h.quota.View()
	if agents == nil {
		agents = []agentquota.AgentQuota{}
	}
	httptransport.SendJSON(w, http.StatusOK, map[string]any{"agents": agents})
}
