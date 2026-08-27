package httphandlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// AgentAuthHandler exposes the shared host-side auth flows registered by the
// agent module catalog. Provider packages configure those flows; HTTP owns only
// route, access-control, and response policy.
type AgentAuthHandler struct {
	bindings []agentauth.Binding
	modules  agentModuleDescriptors
	auth     *serviceauth.Service
}

type agentModuleDescriptors interface {
	Descriptors() []agentmodule.Descriptor
}

func NewAgentAuthHandler(
	bindings []agentauth.Binding,
	auth *serviceauth.Service,
	modules ...agentModuleDescriptors,
) *AgentAuthHandler {
	handler := &AgentAuthHandler{
		bindings: append([]agentauth.Binding(nil), bindings...),
		auth:     auth,
	}
	if len(modules) > 0 {
		handler.modules = modules[0]
	}
	return handler
}

func (h *AgentAuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/agent-auth", h.handleCatalog)
	for _, binding := range h.bindings {
		binding := binding
		prefix := "/api/" + string(binding.ID())
		mux.HandleFunc(prefix+"/auth-status", func(w http.ResponseWriter, r *http.Request) {
			h.handleStatus(binding, w)
		})

		switch binding.Flow() {
		case agentauth.FlowCode:
			mux.HandleFunc(prefix+"/login/start", func(w http.ResponseWriter, r *http.Request) {
				h.handleCodeStart(binding, w, r)
			})
			mux.HandleFunc(prefix+"/login/code", func(w http.ResponseWriter, r *http.Request) {
				h.handleCodeSubmit(binding, w, r)
			})
			mux.HandleFunc(prefix+"/login/cancel", func(w http.ResponseWriter, r *http.Request) {
				h.handleCodeCancel(binding, w, r)
			})
		case agentauth.FlowDevice:
			mux.HandleFunc(prefix+"/login/device", func(w http.ResponseWriter, r *http.Request) {
				h.handleDeviceStart(binding, w, r)
			})
		}
	}
}

type agentAuthCatalogResponse struct {
	Providers []agentAuthProviderResponse `json:"providers"`
}

type agentAuthProviderResponse struct {
	Provider        string                       `json:"provider"`
	Label           string                       `json:"label"`
	Default         bool                         `json:"default,omitempty"`
	ExecutionScopes []agentmodule.ExecutionScope `json:"executionScopes"`
	Authentication  agentAuthPolicyResponse      `json:"authentication"`
	Status          agentauth.Snapshot           `json:"status"`
}

type agentAuthPolicyResponse struct {
	Mode                agentmodule.AuthMode `json:"mode"`
	Instructions        string               `json:"instructions,omitempty"`
	SatisfiesAccessGate bool                 `json:"satisfiesAccessGate"`
}

func (h *AgentAuthHandler) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	bindings := make(map[string]agentauth.Binding, len(h.bindings))
	for _, binding := range h.bindings {
		bindings[string(binding.ID())] = binding
	}
	descriptors := []agentmodule.Descriptor(nil)
	if h.modules != nil {
		descriptors = h.modules.Descriptors()
	}
	providers := make([]agentAuthProviderResponse, 0, len(descriptors))
	for _, descriptor := range descriptors {
		status := agentauth.Snapshot{}
		if descriptor.Auth == agentmodule.AuthNone {
			status.Authenticated = true
		} else if binding, ok := bindings[string(descriptor.ID)]; ok {
			status = binding.Snapshot()
		}
		providers = append(providers, agentAuthProviderResponse{
			Provider:        string(descriptor.ID),
			Label:           descriptor.Label,
			Default:         descriptor.Default,
			ExecutionScopes: append([]agentmodule.ExecutionScope(nil), descriptor.ExecutionScopes...),
			Authentication: agentAuthPolicyResponse{
				Mode:                descriptor.Auth,
				Instructions:        descriptor.AuthInstructions,
				SatisfiesAccessGate: descriptor.SatisfiesAccessGate,
			},
			Status: status,
		})
	}
	httptransport.SendJSON(w, http.StatusOK, agentAuthCatalogResponse{Providers: providers})
}

// Status remains open to every registered user. The outer authentication
// middleware owns that registration gate when user auth is enabled.
func (h *AgentAuthHandler) handleStatus(binding agentauth.Binding, w http.ResponseWriter) {
	httptransport.SendJSON(w, http.StatusOK, binding.Status())
}

func (h *AgentAuthHandler) handleCodeStart(binding agentauth.Binding, w http.ResponseWriter, r *http.Request) {
	if !h.requireMutationAccess(w, r) {
		return
	}

	result, err := binding.StartCode(r.Context())
	if err != nil {
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := map[string]any{"url": result.URL}
	if result.Resumed {
		out["resumed"] = true
	}
	httptransport.SendJSON(w, http.StatusOK, out)
}

func (h *AgentAuthHandler) handleCodeSubmit(binding agentauth.Binding, w http.ResponseWriter, r *http.Request) {
	if !h.requireMutationAccess(w, r) {
		return
	}

	var body struct {
		Code string `json:"code"`
	}
	if err := readJSONBody(r, &body); err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := binding.SubmitCode(r.Context(), body.Code); err != nil {
		status := http.StatusInternalServerError
		if binding.IsCodeInputError(err) {
			status = http.StatusBadRequest
		}
		httptransport.SendErr(w, status, err.Error())
		return
	}
	httptransport.SendJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *AgentAuthHandler) handleCodeCancel(binding agentauth.Binding, w http.ResponseWriter, r *http.Request) {
	if !h.requireMutationAccess(w, r) {
		return
	}
	if err := binding.CancelCode(r.Context()); err != nil {
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httptransport.SendJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *AgentAuthHandler) handleDeviceStart(binding agentauth.Binding, w http.ResponseWriter, r *http.Request) {
	if !h.requireMutationAccess(w, r) {
		return
	}
	state, err := binding.StartDevice(r.Context())
	if err != nil {
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httptransport.SendJSON(w, http.StatusOK, state)
}

func (h *AgentAuthHandler) requireMutationAccess(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	if h.auth == nil {
		return true
	}
	email, err := callerEmailFromRequest(r, h.auth)
	if err != nil || email == "" {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	if ok, _ := h.auth.IsAdmin(r.Context(), email); !ok {
		httptransport.SendErr(w, http.StatusForbidden, "admin only")
		return false
	}
	return true
}

func readJSONBody(r *http.Request, v any) error {
	const max = 1 << 16
	body := http.MaxBytesReader(nil, r.Body, max)
	defer body.Close()
	if err := json.NewDecoder(body).Decode(v); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}
