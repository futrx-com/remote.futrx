package httphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
)

const usageRequestLimit = 256 << 10

// UsageService is the HTTP layer's narrow view of the usage ledger.
type UsageService interface {
	Summary(
		ctx context.Context,
		query serviceusage.Query,
		callerEmail string,
		isAdmin bool,
	) (serviceusage.Summary, error)
	Records(
		ctx context.Context,
		query serviceusage.RecordQuery,
		callerEmail string,
		isAdmin bool,
	) (serviceusage.RecordPage, error)
	Prices(ctx context.Context) (serviceusage.PriceTable, error)
	SetPrices(ctx context.Context, table serviceusage.PriceTable) (serviceusage.PriceTable, error)
	Rebuild(ctx context.Context) (serviceusage.RebuildResult, error)
}

type UsageHandler struct {
	usage UsageService
	auth  *serviceauth.Service
}

func NewUsageHandler(usage UsageService, auth *serviceauth.Service) *UsageHandler {
	return &UsageHandler{usage: usage, auth: auth}
}

func (h *UsageHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/usage/summary", h.HandleSummary)
	mux.HandleFunc("/api/usage/records", h.HandleRecords)
	mux.HandleFunc("/api/admin/usage/prices", h.HandlePrices)
	mux.HandleFunc("/api/admin/usage/rebuild", h.HandleRebuild)
}

func (h *UsageHandler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	email, isAdmin, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}

	query := r.URL.Query()
	from, to, err := usageWindow(query)
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	summary, err := h.usage.Summary(r.Context(), serviceusage.Query{
		From:      from,
		To:        to,
		GroupBy:   serviceusage.NormalizeGroupBy(query.Get("groupBy")),
		ProjectID: strings.TrimSpace(query.Get("projectId")),
		ChatID:    strings.TrimSpace(query.Get("chatId")),
	}, email, isAdmin)
	if err != nil {
		sendUsageError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, summary)
}

// HandleProjectSummary is called by ProjectHandler after it has authorized
// the caller for the project, keeping /api/projects/{id}/usage under the
// existing project route instead of registering an overlapping pattern.
func (h *UsageHandler) HandleProjectSummary(
	w http.ResponseWriter,
	r *http.Request,
	projectID string,
	callerEmail string,
	isAdmin bool,
) {
	if !h.available(w) {
		return
	}
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	from, to, err := usageWindow(r.URL.Query())
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	summary, err := h.usage.Summary(r.Context(), serviceusage.Query{
		From:      from,
		To:        to,
		GroupBy:   serviceusage.GroupByModel,
		ProjectID: projectID,
	}, callerEmail, isAdmin)
	if err != nil {
		sendUsageError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, summary)
}

func (h *UsageHandler) HandleRecords(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	if r.Method != http.MethodGet {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	email, isAdmin, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return
	}

	query := r.URL.Query()
	from, to, err := usageWindow(query)
	if err != nil {
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
		return
	}
	limit := 0
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		parsed, convErr := strconv.Atoi(raw)
		if convErr != nil || parsed < 0 {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	page, err := h.usage.Records(r.Context(), serviceusage.RecordQuery{
		From:      from,
		To:        to,
		ProjectID: strings.TrimSpace(query.Get("projectId")),
		ChatID:    strings.TrimSpace(query.Get("chatId")),
		Limit:     limit,
		Cursor:    strings.TrimSpace(query.Get("cursor")),
	}, email, isAdmin)
	if err != nil {
		sendUsageError(w, err)
		return
	}
	if page.Records == nil {
		page.Records = []serviceusage.Record{}
	}
	httptransport.SendJSON(w, http.StatusOK, page)
}

func (h *UsageHandler) HandlePrices(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	if !h.requireAdmin(w, r) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		table, err := h.usage.Prices(r.Context())
		if err != nil {
			sendUsageError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, table)

	case http.MethodPut:
		var table serviceusage.PriceTable
		if err := json.NewDecoder(io.LimitReader(r.Body, usageRequestLimit)).Decode(&table); err != nil {
			httptransport.SendErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		saved, err := h.usage.SetPrices(r.Context(), table)
		if err != nil {
			sendUsageError(w, err)
			return
		}
		httptransport.SendJSON(w, http.StatusOK, saved)

	default:
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *UsageHandler) HandleRebuild(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	if r.Method != http.MethodPost {
		httptransport.SendErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !h.requireAdmin(w, r) {
		return
	}
	result, err := h.usage.Rebuild(r.Context())
	if err != nil {
		sendUsageError(w, err)
		return
	}
	httptransport.SendJSON(w, http.StatusOK, result)
}

func (h *UsageHandler) available(w http.ResponseWriter) bool {
	if h == nil || h.usage == nil {
		httptransport.SendErr(w, http.StatusServiceUnavailable, "usage ledger unavailable")
		return false
	}
	return true
}

func (h *UsageHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	_, isAdmin, err := h.caller(r)
	if err != nil {
		httptransport.SendErr(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	if !isAdmin {
		httptransport.SendErr(w, http.StatusForbidden, "administrator access required")
		return false
	}
	return true
}

func (h *UsageHandler) caller(r *http.Request) (string, bool, error) {
	if h.auth == nil {
		return "", true, nil
	}
	return httptransport.NewPrincipalResolver(h.auth).EmailAndAdmin(r.Context(), r)
}

// usageWindow reads the from/to query parameters as Unix milliseconds. Both
// are optional; the service defaults an open window to the last 30 days.
func usageWindow(query url.Values) (int64, int64, error) {
	from, err := usageTimestamp(query, "from")
	if err != nil {
		return 0, 0, err
	}
	to, err := usageTimestamp(query, "to")
	if err != nil {
		return 0, 0, err
	}
	return from, to, nil
}

func usageTimestamp(query url.Values, key string) (int64, error) {
	raw := strings.TrimSpace(query.Get(key))
	if raw == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid " + key + " timestamp (expected unix milliseconds)")
	}
	return parsed, nil
}

func sendUsageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, serviceusage.ErrForbidden):
		httptransport.SendErr(w, http.StatusForbidden, err.Error())
	case errors.Is(err, serviceusage.ErrInvalidTime), errors.Is(err, serviceusage.ErrInvalidPrice):
		httptransport.SendErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, serviceusage.ErrUnavailable):
		httptransport.SendErr(w, http.StatusServiceUnavailable, err.Error())
	default:
		httptransport.SendErr(w, http.StatusInternalServerError, err.Error())
	}
}
