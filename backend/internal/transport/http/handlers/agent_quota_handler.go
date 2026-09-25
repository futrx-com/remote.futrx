package httphandlers

import (
	"context"
	"net/http"

	agentquota "github.com/futrx-com/remote.futrx.com/internal/service/agent/quota"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// AgentQuotaService reports each provider account's plan windows, asking the
// providers again when its last answer is out of date.
type AgentQuotaService interface {
	Refresh(ctx context.Context)
	View() []agentquota.AccountView
}

// AgentQuotaHandler serves the Usage tab's plan-limits section.
type AgentQuotaHandler struct {
	quota AgentQuotaService
	auth  *serviceauth.Service
}

// agentQuotaResponse lists readings by provider account. Account IDs are the
// saved-account IDs from the agent-auth snapshot, which owns their labels.
type agentQuotaResponse struct {
	Accounts []agentquota.AccountView `json:"accounts"`
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
// Each request first lets the service ask the providers for current plan
// limits, as Claude Code's /usage and Codex's /status do; the service shares
// one answer between requests for a while. An empty list is a real answer,
// not an error: an install without a subscription login has no plan to show.
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
	h.quota.Refresh(r.Context())
	sendAgentQuota(w, h.quota.View())
}

func sendAgentQuota(w http.ResponseWriter, accounts []agentquota.AccountView) {
	if accounts == nil {
		accounts = []agentquota.AccountView{}
	}
	httptransport.SendJSON(w, http.StatusOK, agentQuotaResponse{Accounts: accounts})
}
