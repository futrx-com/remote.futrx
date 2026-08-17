package httphandlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// AgentAuthHandler exposes the shared host-side auth flows registered by the
// agent catalog. Provider packages configure those flows; HTTP owns only route,
// access-control, and response policy.
type AgentAuthHandler struct {
	bindings []agentauth.Binding
	auth     *serviceauth.Service
	audit    serviceaudit.Recorder
}

func NewAgentAuthHandler(bindings []agentauth.Binding, auth *serviceauth.Service) *AgentAuthHandler {
	return &AgentAuthHandler{
		bindings: append([]agentauth.Binding(nil), bindings...),
		auth:     auth,
	}
}

// WithAudit records host-wide provider credential changes. These tokens are
// shared by every project on the box, so who connected one matters.
func (h *AgentAuthHandler) WithAudit(recorder serviceaudit.Recorder) *AgentAuthHandler {
	h.audit = recorder
	return h
}

// recordAgentAuth writes one provider-credential line. There is no true
// "disconnect" flow today, so cancelling an in-flight login is the closest
// thing and is recorded as one.
func (h *AgentAuthHandler) recordAgentAuth(
	r *http.Request,
	binding agentauth.Binding,
	action, step string,
	err error,
) {
	recordAudit(
		h.audit, r, action,
		serviceaudit.Target{Type: serviceaudit.TargetAgent, ID: string(binding.ID()), Name: string(binding.ID())},
		serviceaudit.Meta{"step": step},
		err,
	)
}

func (h *AgentAuthHandler) RegisterRoutes(mux *http.ServeMux) {
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
	h.recordAgentAuth(r, binding, serviceaudit.ActionSettingsAgentConnect, "start", err)
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
	err := binding.SubmitCode(r.Context(), body.Code)
	h.recordAgentAuth(r, binding, serviceaudit.ActionSettingsAgentConnect, "code", err)
	if err != nil {
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
	err := binding.CancelCode(r.Context())
	h.recordAgentAuth(r, binding, serviceaudit.ActionSettingsAgentDisconnect, "cancel", err)
	if err != nil {
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
	h.recordAgentAuth(r, binding, serviceaudit.ActionSettingsAgentConnect, "device", err)
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
