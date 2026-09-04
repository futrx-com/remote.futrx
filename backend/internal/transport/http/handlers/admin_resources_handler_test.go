package httphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	serviceresources "github.com/futrx-com/remote.futrx.com/internal/service/resources"
)

type stubResourcePolicy struct {
	view    serviceresources.View
	updated []serviceresources.Settings
	actors  []string
	err     error
}

func (s *stubResourcePolicy) Get(context.Context) serviceresources.View { return s.view }

func (s *stubResourcePolicy) Update(
	_ context.Context,
	in serviceresources.Settings,
	actor string,
) (serviceresources.View, error) {
	s.updated = append(s.updated, in)
	s.actors = append(s.actors, actor)
	if s.err != nil {
		return serviceresources.View{}, s.err
	}
	s.view.Settings = in
	return s.view, nil
}

type stubCaller struct {
	email   string
	isAdmin bool
	err     error
}

func (c stubCaller) EmailAndAdmin(context.Context, *http.Request) (string, bool, error) {
	return c.email, c.isAdmin, c.err
}

func newResourcesMux(policy ResourcePolicyService, caller CallerResolver) *http.ServeMux {
	mux := http.NewServeMux()
	NewAdminResourcesHandler(policy, caller).RegisterRoutes(mux)
	return mux
}

func TestAdminResourcesRequiresAnAdminCaller(t *testing.T) {
	tests := []struct {
		name   string
		method string
		caller CallerResolver
		want   int
	}{
		{name: "anonymous GET", method: http.MethodGet, caller: stubCaller{}, want: http.StatusUnauthorized},
		{name: "anonymous PUT", method: http.MethodPut, caller: stubCaller{}, want: http.StatusUnauthorized},
		{
			name:   "broken session",
			method: http.MethodGet,
			caller: stubCaller{email: "member@example.com", err: errors.New("bad cookie")},
			want:   http.StatusUnauthorized,
		},
		{
			name:   "member GET",
			method: http.MethodGet,
			caller: stubCaller{email: "member@example.com"},
			want:   http.StatusForbidden,
		},
		{
			name:   "member PUT",
			method: http.MethodPut,
			caller: stubCaller{email: "member@example.com"},
			want:   http.StatusForbidden,
		},
		{
			name:   "admin GET",
			method: http.MethodGet,
			caller: stubCaller{email: "admin@example.com", isAdmin: true},
			want:   http.StatusOK,
		},
		{
			name:   "admin PUT",
			method: http.MethodPut,
			caller: stubCaller{email: "admin@example.com", isAdmin: true},
			want:   http.StatusOK,
		},
		{
			name:   "no resolver at all",
			method: http.MethodGet,
			caller: nil,
			want:   http.StatusForbidden,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mux := newResourcesMux(&stubResourcePolicy{}, test.caller)
			body := strings.NewReader(`{"defaults":{"memory":"2GiB","cpu":2,"processes":2000}}`)
			request := httptest.NewRequest(test.method, "/api/admin/resources", body)
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, request)

			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d (body %q)", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestAdminResourcesRejectsOtherMethods(t *testing.T) {
	mux := newResourcesMux(&stubResourcePolicy{}, stubCaller{email: "admin@example.com", isAdmin: true})

	for _, method := range []string{http.MethodPost, http.MethodDelete, http.MethodPatch} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(method, "/api/admin/resources", nil))
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want 405", method, recorder.Code)
		}
	}
}

func TestAdminResourcesPutPassesSettingsAndActor(t *testing.T) {
	policy := &stubResourcePolicy{}
	mux := newResourcesMux(policy, stubCaller{email: "admin@example.com", isAdmin: true})

	body := strings.NewReader(
		`{"defaults":{"memory":"3GiB","cpu":2,"processes":2000,"disk":"25GiB"},` +
			`"hostReserve":{"memory":"1GiB","cpu":0.5},"maxRunningContainers":4}`,
	)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/admin/resources", body))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", recorder.Code, recorder.Body.String())
	}
	if len(policy.updated) != 1 {
		t.Fatalf("expected one update, got %d", len(policy.updated))
	}
	got := policy.updated[0]
	if got.Defaults.Memory != "3GiB" || got.Defaults.Disk != "25GiB" || got.MaxRunningContainers != 4 {
		t.Fatalf("decoded settings = %+v", got)
	}
	if policy.actors[0] != "admin@example.com" {
		t.Fatalf("actor = %q, want the caller email", policy.actors[0])
	}
}

func TestAdminResourcesPutMapsValidationErrorsTo400(t *testing.T) {
	policy := &stubResourcePolicy{err: serviceresources.ErrInvalidSettings}
	mux := newResourcesMux(policy, stubCaller{email: "admin@example.com", isAdmin: true})

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodPut,
		"/api/admin/resources",
		strings.NewReader(`{"defaults":{"memory":"1MiB"}}`),
	))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestAdminResourcesPutRejectsMalformedJSON(t *testing.T) {
	mux := newResourcesMux(&stubResourcePolicy{}, stubCaller{email: "admin@example.com", isAdmin: true})

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodPut,
		"/api/admin/resources",
		strings.NewReader(`{`),
	))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestAdminResourcesGetReturnsTheView(t *testing.T) {
	policy := &stubResourcePolicy{view: serviceresources.View{
		Settings: serviceresources.Settings{
			Defaults: serviceresources.Limits{Memory: "2GiB", CPU: 2, Processes: 2000, Disk: "20GiB"},
		},
		Host:      serviceresources.HostCapacity{MemoryBytes: 4 << 30, CPUs: 1},
		DiskQuota: serviceresources.PoolCapability{Pool: "default", Driver: "dir"},
		Available: true,
	}}
	mux := newResourcesMux(policy, stubCaller{email: "admin@example.com", isAdmin: true})

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/resources", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var got serviceresources.View
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Settings.Defaults.Memory != "2GiB" || got.DiskQuota.Driver != "dir" || !got.Available {
		t.Fatalf("view = %+v", got)
	}
}

func TestAdminResourcesWithoutAServiceReports503(t *testing.T) {
	mux := newResourcesMux(nil, stubCaller{email: "admin@example.com", isAdmin: true})

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/resources", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}
