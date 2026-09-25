package httphandlers

import (
	"net/http"

	agentquota "github.com/futrx-com/remote.futrx.com/internal/service/agent/quota"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// AgentQuotaService reports the last subscription windows each provider
// account mentioned.
type AgentQuotaService interface {
	View() []agentquota.AccountQuota
}

// AgentQuotaHandler serves the Usage tab's plan-limits section.
type AgentQuotaHandler struct {
	quota AgentQuotaService
	auth  *serviceauth.Service
}

// agentQuotaResponse lists readings by provider account. Account IDs are the
// saved-account IDs from the agent-auth snapshot, which owns their labels.
type agentQuotaResponse struct {
	Accounts []agentquota.AccountQuota `json:"accounts"`
}

func NewAgentQuotaHandler(quota AgentQuotaService, auth *serviceauth.Service) *AgentQuotaHandler {
	return &AgentQuotaHandler{quota: quota, auth: auth}
}

func (h *AgentQuotaHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/agent-quota", h.handle)
}

// handle answers any signed-in user, or any caller on an install without
// application authentication.
//
// An empty list is a real answer, not an error: readings only arrive while an
// agent runs, so a platform nobody has used yet genuinely knows nothing. The
// browser hides the section until an account has reported a window.
func (h *AgentQuotaHandler) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h == nil || h.quota == nil {
		sendAgentQuota(w, nil)
		return
	}
	if h.auth != nil {
		email, err := httptransport.NewPrincipalResolver(h.auth).Email(r)
		if err != nil || email == "" {
			httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
	}
	sendAgentQuota(w, h.quota.View())
}

func sendAgentQuota(w http.ResponseWriter, accounts []agentquota.AccountQuota) {
	if accounts == nil {
		accounts = []agentquota.AccountQuota{}
	}
	httptransport.SendJSON(w, http.StatusOK, agentQuotaResponse{Accounts: accounts})
}
