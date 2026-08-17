package httphandlers

import (
	"net/http"

	servicetemplates "github.com/futrx-com/remote.futrx.com/internal/service/container/templates"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// TemplateHandler serves the project-template catalog used by the
// new-project dialog. The list is static per build; only the "is a pre-built
// image published on this host" flag is resolved per request.
type TemplateHandler struct {
	templates *servicetemplates.Service
}

func NewTemplateHandler(templates *servicetemplates.Service) *TemplateHandler {
	return &TemplateHandler{templates: templates}
}

func (h *TemplateHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/templates", h.HandleCollection)
}

func (h *TemplateHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.templates == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "templates unavailable")
		return
	}
	items := h.templates.List(r.Context())
	if items == nil {
		items = []servicetemplates.Descriptor{}
	}
	httptransport.SendJSON(w, http.StatusOK, items)
}
