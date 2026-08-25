package httphandlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
)

func TestProjectHandlerTLSAskUsesConfiguredPublicHostname(t *testing.T) {
	handler, project := newTLSAskProjectHandler(t, "remote.example.test")
	tests := []struct {
		name       string
		domain     string
		wantStatus int
	}{
		{
			name:       "configured preview host",
			domain:     project.Slug + "--3000.dev.remote.example.test",
			wantStatus: http.StatusOK,
		},
		{
			name:       "configured code host",
			domain:     project.Slug + ".code.remote.example.test",
			wantStatus: http.StatusOK,
		},
		{
			name:       "previous hard-coded host",
			domain:     project.Slug + "--3000.dev.remote.futrx.com",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "deceptive suffix",
			domain:     project.Slug + "--3000.dev.remote.example.test.attacker.test",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "port below range",
			domain:     project.Slug + "--1023.dev.remote.example.test",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "port above range",
			domain:     project.Slug + "--65536.dev.remote.example.test",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unknown project",
			domain:     "missing--3000.dev.remote.example.test",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodGet,
				"/internal/tls-ask?domain="+url.QueryEscape(test.domain),
				nil,
			)
			rec := httptest.NewRecorder()

			handler.HandleTLSAsk(rec, req)

			if rec.Code != test.wantStatus {
				t.Fatalf("HandleTLSAsk(%q) = %d, want %d; body=%q", test.domain, rec.Code, test.wantStatus, rec.Body.String())
			}
		})
	}
}

func newTLSAskProjectHandler(t *testing.T, publicHostname string) (*ProjectHandler, serviceproject.Meta) {
	t.Helper()
	repo, err := fileproject.NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projects := serviceproject.New(repo, serviceproject.ContainerDependencies{}, nil, nil)
	project, err := projects.Create(
		context.Background(),
		serviceproject.CreateInput{Name: "TLS Ask Project"},
		"user@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	return NewProjectHandler(projects, nil, nil, nil, publicHostname), project
}
