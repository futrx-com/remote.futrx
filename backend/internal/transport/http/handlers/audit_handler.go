package httphandlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

// AuditHandler exposes the audit log to administrators. Both routes re-check
// IsAdmin: the API gate only proves the caller is registered, and the trail
// names every user on the box.
type AuditHandler struct {
	audit *serviceaudit.Service
	auth  *serviceauth.Service
}

func NewAuditHandler(auditLog *serviceaudit.Service, auth *serviceauth.Service) *AuditHandler {
	return &AuditHandler{audit: auditLog, auth: auth}
}

func (h *AuditHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/audit", h.handleQuery)
	mux.HandleFunc("/api/admin/audit/export", h.handleExport)
}

func (h *AuditHandler) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !h.requireAdmin(w, r) {
		return
	}

	query := serviceaudit.Query{
		Actor:  strings.TrimSpace(r.URL.Query().Get("actor")),
		Action: strings.TrimSpace(r.URL.Query().Get("action")),
		Target: strings.TrimSpace(r.URL.Query().Get("target")),
		Limit:  intQuery(r, "limit", 0),
		Cursor: strings.TrimSpace(r.URL.Query().Get("cursor")),
	}
	from, err := parseAuditTime(r.URL.Query().Get("from"))
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid from timestamp")
		return
	}
	to, err := parseAuditTime(r.URL.Query().Get("to"))
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid to timestamp")
		return
	}
	query.From, query.To = from, to

	page, err := h.audit.Query(r.Context(), query)
	if err != nil {
		sendAuditError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, page)
}

// handleExport streams the stored JSONL for a range straight to the client, so
// an operator can archive or grep the raw trail without paging the UI.
func (h *AuditHandler) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !h.requireAdmin(w, r) {
		return
	}

	from, err := parseAuditTime(r.URL.Query().Get("from"))
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid from timestamp")
		return
	}
	to, err := parseAuditTime(r.URL.Query().Get("to"))
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, "invalid to timestamp")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", `attachment; filename="audit-log.jsonl"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := h.audit.Export(r.Context(), from, to, w); err != nil && r.Context().Err() == nil {
		// A streamed response cannot change its status once bytes are on the
		// wire, so a mid-export failure shows up as a truncated download plus
		// this log line.
		log.Printf("audit: export failed: %v", err)
	}
}

func (h *AuditHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if h.audit == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "audit log unavailable")
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
	if admin, _ := h.auth.IsAdmin(r.Context(), email); !admin {
		httptransport.SendErr(w, http.StatusForbidden, "admin only")
		return false
	}
	return true
}

// parseAuditTime accepts RFC3339 (what the browser sends) or unix
// milliseconds (what a script is likely to have). An empty value means
// "unbounded".
func parseAuditTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC(), nil
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(millis).UTC(), nil
}

func sendAuditError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, serviceaudit.ErrStoreUnavailable):
		httptransport.SendErr(w, http.StatusServiceUnavailable, err.Error())
	default:
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	}
}
