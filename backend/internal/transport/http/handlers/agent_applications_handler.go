package httphandlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	serviceapplications "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type AgentApplications interface {
	CallAgentApplication(context.Context, string, string, applications.Request) (applications.Response, error)
}
type AgentApplicationsHandler struct{ apps AgentApplications }

func NewAgentApplicationsHandler(apps AgentApplications) *AgentApplicationsHandler {
	return &AgentApplicationsHandler{apps}
}
func (h *AgentApplicationsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/agent-api/applications/", h.handle)
}
func (h *AgentApplicationsHandler) handle(w http.ResponseWriter, r *http.Request) {
	if h.apps == nil {
		httptransport.SendErr(w, 503, "application tools unavailable")
		return
	}
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || scheme != "Bearer" || token == "" {
		httptransport.SendErr(w, 401, "capability required")
		return
	}
	appID, path, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/agent-api/applications/"), "/")
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		httptransport.SendErr(w, 400, "body exceeds 64 KiB")
		return
	}
	response, err := h.apps.CallAgentApplication(r.Context(), token, appID, applications.Request{Method: r.Method, Path: path, Query: r.URL.Query(), Body: body})
	if err != nil {
		status := 503
		if errors.Is(err, serviceapplications.ErrInvalidAgentGrant) {
			status = 401
		} else if errors.Is(err, applications.ErrAgentAccess) {
			status = 403
		}
		httptransport.SendErr(w, status, err.Error())
		return
	}
	writeBackendResponse(w, response)
}
