package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileaudit"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
)

type auditTestDirectory struct {
	admins map[string]bool
}

func (d auditTestDirectory) IsAdmin(_ context.Context, email string) (bool, error) {
	return d.admins[email], nil
}

func (d auditTestDirectory) IsRegistered(_ context.Context, email string) (bool, error) {
	_, ok := d.admins[email]
	return ok, nil
}

func (auditTestDirectory) AddBootstrapAdmin(context.Context, string) error { return nil }

func (auditTestDirectory) FirstAdmin(context.Context) (*serviceauth.UserDirectoryEntry, error) {
	return &serviceauth.UserDirectoryEntry{Email: "admin@example.com"}, nil
}

type auditTestOAuth struct{}

func (auditTestOAuth) AuthCodeURL(string) string { return "https://accounts.example.com" }
func (auditTestOAuth) ExchangeUser(context.Context, string) (serviceauth.User, error) {
	return serviceauth.User{}, nil
}

func newAuditTestHandler(t *testing.T) (*http.ServeMux, *serviceauth.Service, *serviceaudit.Service) {
	t.Helper()
	auth, err := serviceauth.New(
		context.Background(),
		fileauth.New(t.TempDir()),
		auditTestDirectory{admins: map[string]bool{
			"admin@example.com":  true,
			"member@example.com": false,
		}},
		func(string, string, string) serviceauth.OAuthProvider { return auditTestOAuth{} },
		"https://remote.example.com",
		[]byte("test-session-key"),
	)
	if err != nil {
		t.Fatalf("New auth service: %v", err)
	}
	store, err := fileaudit.New(t.TempDir())
	if err != nil {
		t.Fatalf("New audit store: %v", err)
	}
	auditLog := serviceaudit.New(store)
	mux := http.NewServeMux()
	NewAuditHandler(auditLog, auth).RegisterRoutes(mux)
	return mux, auth, auditLog
}

func auditRequest(mux *http.ServeMux, auth *serviceauth.Service, method, path, email string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if email != "" {
		req.AddCookie(&http.Cookie{
			Name:  serviceauth.SessionCookieName,
			Value: auth.SignSession(serviceauth.User{Email: email, Sub: "sub-" + email}),
		})
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestAuditRoutesAreAdminOnly(t *testing.T) {
	mux, auth, _ := newAuditTestHandler(t)

	tests := []struct {
		name  string
		path  string
		email string
		want  int
	}{
		{name: "query without a session", path: "/api/admin/audit", email: "", want: http.StatusUnauthorized},
		{name: "query as a member", path: "/api/admin/audit", email: "member@example.com", want: http.StatusForbidden},
		{name: "query as an admin", path: "/api/admin/audit", email: "admin@example.com", want: http.StatusOK},
		{name: "export without a session", path: "/api/admin/audit/export", email: "", want: http.StatusUnauthorized},
		{name: "export as a member", path: "/api/admin/audit/export", email: "member@example.com", want: http.StatusForbidden},
		{name: "export as an admin", path: "/api/admin/audit/export", email: "admin@example.com", want: http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := auditRequest(mux, auth, http.MethodGet, tc.path, tc.email)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestAuditRoutesRejectNonGetMethods(t *testing.T) {
	mux, auth, _ := newAuditTestHandler(t)

	for _, path := range []string{"/api/admin/audit", "/api/admin/audit/export"} {
		for _, method := range []string{http.MethodPost, http.MethodDelete, http.MethodPut} {
			rec := auditRequest(mux, auth, method, path, "admin@example.com")
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s = %d, want %d", method, path, rec.Code, http.StatusMethodNotAllowed)
			}
		}
	}
}

func TestAuditQueryAppliesFiltersAndPaginates(t *testing.T) {
	mux, auth, auditLog := newAuditTestHandler(t)

	base := time.Now().UTC().Add(-time.Hour)
	for i, action := range []string{"project.create", "project.secret.read", "auth.login.success"} {
		entry := serviceaudit.Success(action, serviceaudit.Target{Type: "project", ID: "p1"}, nil)
		entry.At = base.Add(time.Duration(i) * time.Minute)
		entry.Actor = serviceaudit.Actor{Email: "admin@example.com"}
		auditLog.Record(context.Background(), entry)
	}

	rec := auditRequest(mux, auth, http.MethodGet, "/api/admin/audit?action=project.&limit=1", "admin@example.com")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	var page serviceaudit.Page
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Action != "project.secret.read" {
		t.Fatalf("entries = %+v, want the newest project action first", page.Entries)
	}
	if page.NextCursor == "" {
		t.Fatal("NextCursor is empty, want a resumable page")
	}

	rec = auditRequest(
		mux, auth, http.MethodGet,
		"/api/admin/audit?action=project.&limit=1&cursor="+page.NextCursor,
		"admin@example.com",
	)
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode page 2: %v", err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Action != "project.create" {
		t.Fatalf("second page = %+v", page.Entries)
	}
}

func TestAuditQueryRejectsUnparseableTimeBounds(t *testing.T) {
	mux, auth, _ := newAuditTestHandler(t)

	for _, query := range []string{"?from=yesterday", "?to=soon"} {
		rec := auditRequest(mux, auth, http.MethodGet, "/api/admin/audit"+query, "admin@example.com")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status for %s = %d, want %d", query, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestAuditExportStreamsJSONLAttachment(t *testing.T) {
	mux, auth, auditLog := newAuditTestHandler(t)

	entry := serviceaudit.Success("project.create", serviceaudit.Target{Type: "project", ID: "p1"}, nil)
	entry.Actor = serviceaudit.Actor{Email: "admin@example.com"}
	auditLog.Record(context.Background(), entry)

	rec := auditRequest(mux, auth, http.MethodGet, "/api/admin/audit/export", "admin@example.com")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/x-ndjson" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "audit-log.jsonl") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	body := strings.TrimSpace(rec.Body.String())
	if !strings.Contains(body, `"action":"project.create"`) {
		t.Fatalf("export body = %q", body)
	}
	if strings.Count(body, "\n") != 0 {
		t.Fatalf("export returned %d lines, want exactly one", strings.Count(body, "\n")+1)
	}
}

func TestParseAuditTimeAcceptsRFC3339AndMillis(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want time.Time
		fail bool
	}{
		{name: "empty is unbounded", raw: "", want: time.Time{}},
		{name: "rfc3339", raw: "2026-08-17T10:00:00Z", want: time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)},
		{name: "unix millis", raw: "1755424800000", want: time.UnixMilli(1755424800000).UTC()},
		{name: "garbage", raw: "not-a-time", fail: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAuditTime(tc.raw)
			if tc.fail {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAuditTime: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("time = %s, want %s", got, tc.want)
			}
		})
	}
}
