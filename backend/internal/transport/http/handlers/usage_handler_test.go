package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
)

type stubUsageService struct {
	summaryQuery serviceusage.Query
	recordQuery  serviceusage.RecordQuery
	savedPrices  serviceusage.PriceTable
	rebuilds     int
	summaryErr   error
}

func (s *stubUsageService) Summary(
	_ context.Context,
	query serviceusage.Query,
	_ string,
	_ bool,
) (serviceusage.Summary, error) {
	s.summaryQuery = query
	if s.summaryErr != nil {
		return serviceusage.Summary{}, s.summaryErr
	}
	return serviceusage.Summary{From: query.From, To: query.To, GroupBy: query.GroupBy}, nil
}

func (s *stubUsageService) Records(
	_ context.Context,
	query serviceusage.RecordQuery,
	_ string,
	_ bool,
) (serviceusage.RecordPage, error) {
	s.recordQuery = query
	return serviceusage.RecordPage{}, nil
}

func (s *stubUsageService) Prices(context.Context) (serviceusage.PriceTable, error) {
	return serviceusage.DefaultPriceTable(), nil
}

func (s *stubUsageService) SetPrices(
	_ context.Context,
	table serviceusage.PriceTable,
) (serviceusage.PriceTable, error) {
	s.savedPrices = table
	return table, nil
}

func (s *stubUsageService) Rebuild(context.Context) (serviceusage.RebuildResult, error) {
	s.rebuilds++
	return serviceusage.RebuildResult{Records: 3}, nil
}

// A nil auth service means "no authentication configured", which the handler
// treats as an administrator so tests can exercise every route.
func newTestUsageHandler(usage UsageService) *UsageHandler {
	return NewUsageHandler(usage, nil)
}

func TestUsageSummaryParsesQuery(t *testing.T) {
	stub := &stubUsageService{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/usage/summary?from=1000&to=2000&groupBy=provider&projectId=abcd1234",
		nil,
	)
	newTestUsageHandler(stub).HandleSummary(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body)
	}
	if stub.summaryQuery.From != 1000 || stub.summaryQuery.To != 2000 {
		t.Fatalf("window = %d..%d", stub.summaryQuery.From, stub.summaryQuery.To)
	}
	if stub.summaryQuery.GroupBy != serviceusage.GroupByProvider {
		t.Fatalf("groupBy = %q", stub.summaryQuery.GroupBy)
	}
	if stub.summaryQuery.ProjectID != "abcd1234" {
		t.Fatalf("projectId = %q", stub.summaryQuery.ProjectID)
	}
}

func TestUsageSummaryRejectsBadInput(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		method string
		err    error
		status int
	}{
		{name: "non numeric from", url: "/api/usage/summary?from=yesterday", status: http.StatusBadRequest},
		{name: "negative to", url: "/api/usage/summary?to=-5", status: http.StatusBadRequest},
		{
			name:   "method not allowed",
			url:    "/api/usage/summary",
			method: http.MethodPost,
			status: http.StatusMethodNotAllowed,
		},
		{
			name:   "forbidden project",
			url:    "/api/usage/summary?projectId=other",
			err:    serviceusage.ErrForbidden,
			status: http.StatusForbidden,
		},
		{
			name:   "inverted window",
			url:    "/api/usage/summary?from=9&to=1",
			err:    serviceusage.ErrInvalidTime,
			status: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			method := test.method
			if method == "" {
				method = http.MethodGet
			}
			recorder := httptest.NewRecorder()
			handler := newTestUsageHandler(&stubUsageService{summaryErr: test.err})
			handler.HandleSummary(recorder, httptest.NewRequest(method, test.url, nil))
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.status, recorder.Body)
			}
		})
	}
}

func TestUsageRecordsAlwaysReturnsAnArray(t *testing.T) {
	stub := &stubUsageService{}
	recorder := httptest.NewRecorder()
	newTestUsageHandler(stub).HandleRecords(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/usage/records?chatId=abcd&limit=25", nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body)
	}
	var body struct {
		Records []serviceusage.Record `json:"records"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Records == nil {
		t.Fatalf("records must serialize as [] not null: %s", recorder.Body)
	}
	if stub.recordQuery.ChatID != "abcd" || stub.recordQuery.Limit != 25 {
		t.Fatalf("query = %+v", stub.recordQuery)
	}
}

func TestUsagePricesAndRebuild(t *testing.T) {
	stub := &stubUsageService{}
	handler := newTestUsageHandler(stub)

	recorder := httptest.NewRecorder()
	handler.HandlePrices(recorder, httptest.NewRequest(
		http.MethodPut,
		"/api/admin/usage/prices",
		strings.NewReader(`{"currency":"USD","models":[{"match":"x","inputPerMTok":1,"outputPerMTok":2}]}`),
	))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body)
	}
	if len(stub.savedPrices.Models) != 1 || stub.savedPrices.Models[0].Match != "x" {
		t.Fatalf("saved prices = %+v", stub.savedPrices)
	}

	recorder = httptest.NewRecorder()
	handler.HandlePrices(recorder, httptest.NewRequest(
		http.MethodPut, "/api/admin/usage/prices", strings.NewReader("not json"),
	))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handler.HandleRebuild(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/usage/rebuild", nil))
	if recorder.Code != http.StatusOK || stub.rebuilds != 1 {
		t.Fatalf("status = %d, rebuilds = %d", recorder.Code, stub.rebuilds)
	}

	recorder = httptest.NewRecorder()
	handler.HandleRebuild(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/usage/rebuild", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}

func TestUsageHandlerWithoutServiceIsUnavailable(t *testing.T) {
	recorder := httptest.NewRecorder()
	newTestUsageHandler(nil).HandleSummary(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/usage/summary", nil),
	)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

// The ledger routes share the /api/admin prefix with users and updates, so
// this pins that they register on one mux without a pattern conflict and that
// each path resolves to the intended handler.
func TestUsageRoutesRegisterAlongsideProjectRoutes(t *testing.T) {
	mux := http.NewServeMux()
	stub := &stubUsageService{}
	usage := newTestUsageHandler(stub)
	usage.RegisterRoutes(mux)
	NewProjectHandler(nil, nil, nil, "example.com").WithUsage(usage).RegisterRoutes(mux)

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/usage/summary"},
		{http.MethodGet, "/api/usage/records"},
		{http.MethodGet, "/api/admin/usage/prices"},
		{http.MethodPost, "/api/admin/usage/rebuild"},
	} {
		request := httptest.NewRequest(route.method, route.path, nil)
		handler, pattern := mux.Handler(request)
		if pattern != route.path {
			t.Fatalf("%s resolved to pattern %q", route.path, pattern)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d: %s", route.path, recorder.Code, recorder.Body)
		}
	}
}
