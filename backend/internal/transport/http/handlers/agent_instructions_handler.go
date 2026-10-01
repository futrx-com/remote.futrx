package httphandlers

import (
	"encoding/json"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
	"io"
	"net/http"
)

type AgentInstructionsStore interface {
	Read() (json.RawMessage, error)
	Write([]byte) error
	Targets() map[string]string
}
type AgentInstructionsHandler struct {
	store AgentInstructionsStore
	auth  *serviceauth.Service
}

func NewAgentInstructionsHandler(store AgentInstructionsStore, auth *serviceauth.Service) *AgentInstructionsHandler {
	return &AgentInstructionsHandler{store, auth}
}
func (h *AgentInstructionsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/agent-instructions", h.handle)
}
func (h *AgentInstructionsHandler) handle(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil || h.store == nil {
		httptransport.SendErr(w, 503, "instructions settings unavailable")
		return
	}
	email, err := callerEmailFromRequest(r, h.auth)
	if err != nil || email == "" {
		httptransport.SendErr(w, 401, "authentication required")
		return
	}
	admin, err := h.auth.IsAdmin(r.Context(), email)
	if err != nil || !admin {
		httptransport.SendErr(w, 403, "admin only")
		return
	}
	switch r.Method {
	case http.MethodGet:
		data, err := h.store.Read()
		if err != nil {
			httptransport.SendErr(w, 500, "cannot read instructions settings")
			return
		}
		httptransport.SendJSON(w, 200, map[string]any{"instructions": data, "targets": h.store.Targets()})
	case http.MethodPut:
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024*1024))
		if err != nil {
			httptransport.SendErr(w, 400, "instructions must be at most 1 MiB")
			return
		}
		if err := h.store.Write(data); err != nil {
			httptransport.SendErr(w, 400, "cannot save instructions: check JSON fields, provider IDs and storage permissions")
			return
		}
		httptransport.SendJSON(w, 200, map[string]bool{"restartRequired": true})
	default:
		w.Header().Set("Allow", "GET, PUT")
		httptransport.SendErr(w, 405, "method not allowed")
	}
}
