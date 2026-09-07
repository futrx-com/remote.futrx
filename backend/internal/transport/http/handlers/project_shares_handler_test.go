package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceshare "github.com/futrx-com/remote.futrx.com/internal/service/share"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileprojectshares"
)

func TestProjectSharesLifecycle(t *testing.T) {
	handler, project := newSharesProjectHandler(t)
	base := "/api/projects/" + string(project.ID) + "/shares"

	created := decodeShare(t, sharesRequest(t, handler, http.MethodPost, base,
		`{"port":3000,"ttlHours":168,"label":"client demo"}`, http.StatusCreated))

	wantPrefix := "https://" + project.Slug + "--3000.dev." + sharesPublicHostname + "/?share="
	if !strings.HasPrefix(created.URL, wantPrefix) {
		t.Fatalf("url = %q, want prefix %q", created.URL, wantPrefix)
	}
	token := strings.TrimPrefix(created.URL, wantPrefix)
	if len(token) < 43 {
		t.Fatalf("token in url = %q, want a full-entropy token", token)
	}
	if created.ExpiresAt-created.CreatedAt != 168*60*60*1000 {
		t.Fatalf("lifetime = %d ms, want 168h", created.ExpiresAt-created.CreatedAt)
	}

	listed := decodeShares(t, sharesRequest(t, handler, http.MethodGet, base, "", http.StatusOK))
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("list = %#v, want the created link", listed)
	}
	if listed[0].URL != "" {
		t.Fatalf("list exposed a url (%q); the token must be shown only once", listed[0].URL)
	}

	sharesRequest(t, handler, http.MethodDelete, base+"/"+created.ID, "", http.StatusOK)

	listed = decodeShares(t, sharesRequest(t, handler, http.MethodGet, base, "", http.StatusOK))
	if len(listed) != 0 {
		t.Fatalf("list after revoke = %#v, want empty", listed)
	}
	sharesRequest(t, handler, http.MethodDelete, base+"/"+created.ID, "", http.StatusNotFound)
}

func TestProjectSharesRejectBadRequests(t *testing.T) {
	handler, project := newSharesProjectHandler(t)
	base := "/api/projects/" + string(project.ID) + "/shares"

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			name: "agent browser port", method: http.MethodPost, path: base,
			body: `{"port":6080}`, wantStatus: http.StatusBadRequest,
		},
		{
			name: "port below range", method: http.MethodPost, path: base,
			body: `{"port":80}`, wantStatus: http.StatusBadRequest,
		},
		{
			name: "ttl beyond 30 days", method: http.MethodPost, path: base,
			body: `{"port":3000,"ttlHours":721}`, wantStatus: http.StatusBadRequest,
		},
		{
			name: "malformed body", method: http.MethodPost, path: base,
			body: `{`, wantStatus: http.StatusBadRequest,
		},
		{
			name: "unsupported method", method: http.MethodPut, path: base,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name: "unknown share id", method: http.MethodDelete, path: base + "/deadbeef",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "unknown project", method: http.MethodGet,
			path: "/api/projects/ffffffff/shares", wantStatus: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sharesRequest(t, handler, test.method, test.path, test.body, test.wantStatus)
		})
	}
}

func TestProjectSharesReportUnavailableWithoutStore(t *testing.T) {
	repo, err := fileproject.NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projects := serviceproject.New(repo, serviceproject.ContainerDependencies{}, nil, nil)
	project, err := projects.Create(
		context.Background(), serviceproject.CreateInput{Name: "No Shares"}, "owner@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewProjectHandler(projects, nil, nil, sharesPublicHostname)

	sharesRequest(
		t, handler, http.MethodGet,
		"/api/projects/"+string(project.ID)+"/shares", "", http.StatusServiceUnavailable,
	)
}

func sharesRequest(
	t *testing.T,
	handler *ProjectHandler,
	method, path, body string,
	wantStatus int,
) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Host = sharesPublicHostname
	rec := httptest.NewRecorder()
	handler.HandleResource(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s = %d, want %d; body=%s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	return rec
}

func decodeShare(t *testing.T, rec *httptest.ResponseRecorder) shareResponse {
	t.Helper()
	var out shareResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode share: %v", err)
	}
	return out
}

func decodeShares(t *testing.T, rec *httptest.ResponseRecorder) []shareResponse {
	t.Helper()
	var out []shareResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode shares: %v", err)
	}
	return out
}

const sharesPublicHostname = "remote.example.test"

func newSharesProjectHandler(t *testing.T) (*ProjectHandler, serviceproject.Meta) {
	t.Helper()
	dataDir := t.TempDir()
	repo, err := fileproject.NewWithWorkspaceRoot(dataDir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projects := serviceproject.New(repo, serviceproject.ContainerDependencies{}, nil, nil)
	project, err := projects.Create(
		context.Background(), serviceproject.CreateInput{Name: "Share Project"}, "owner@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	shareStore, err := fileprojectshares.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewProjectHandler(projects, nil, nil, sharesPublicHostname).
		WithShares(serviceshare.New(shareStore, projects))
	return handler, project
}

// TestShareURLTokenIsQueryEscaped guards the one place a token is rendered
// into a URL.
func TestShareURLTokenIsQueryEscaped(t *testing.T) {
	handler, project := newSharesProjectHandler(t)
	got := handler.shareURL(project.Slug, 3000, "a+b/c=")
	want := "https://" + project.Slug + "--3000.dev." + sharesPublicHostname +
		"/?share=" + url.QueryEscape("a+b/c=")
	if got != want {
		t.Fatalf("shareURL = %q, want %q", got, want)
	}
}
