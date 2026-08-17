package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
)

// stubTemplateCatalog stands in for the template service in transport tests.
type stubTemplateCatalog struct{ known map[string]bool }

func (s stubTemplateCatalog) Has(name string) bool { return s.known[name] }

func (s stubTemplateCatalog) DefaultName() string { return serviceproject.DefaultTemplate }

func (s stubTemplateCatalog) TemplateStatus(
	_ context.Context,
	_, template string,
) serviceproject.TemplateStatus {
	return serviceproject.TemplateStatus{Name: template, Status: "pending"}
}

func (s stubTemplateCatalog) ForgetTemplateState(string) {}

func newTemplateProjectHandler(t *testing.T) *ProjectHandler {
	t.Helper()
	repo, err := fileproject.NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	containers := newFakeProjectContainers()
	projects := serviceproject.New(repo, serviceproject.ContainerDependencies{
		Lifecycle: containers,
		Inspector: containers,
		Templates: stubTemplateCatalog{known: map[string]bool{"blank": true, "wordpress": true}},
	}, nil, nil)
	return NewProjectHandler(projects, nil, nil, "remote.futrx.com")
}

func TestProjectCreateAcceptsATemplate(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
		want     string
	}{
		{
			name:     "explicit template",
			body:     `{"name":"Shop","template":"wordpress"}`,
			wantCode: http.StatusCreated,
			want:     "wordpress",
		},
		{
			name:     "omitted template takes the default",
			body:     `{"name":"Plain"}`,
			wantCode: http.StatusCreated,
			want:     serviceproject.DefaultTemplate,
		},
		{
			name:     "unknown template is a client error",
			body:     `{"name":"Nope","template":"cobol"}`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newTemplateProjectHandler(t)
			request := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(tt.body))
			response := httptest.NewRecorder()

			handler.HandleCollection(response, request)

			if response.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantCode, response.Body)
			}
			if tt.wantCode != http.StatusCreated {
				return
			}
			var meta serviceproject.Meta
			if err := json.NewDecoder(response.Body).Decode(&meta); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if meta.Template != tt.want {
				t.Fatalf("template = %q, want %q", meta.Template, tt.want)
			}
		})
	}
}

func TestProjectContainerRouteReportsTemplateProvisioning(t *testing.T) {
	handler := newTemplateProjectHandler(t)
	request := httptest.NewRequest(
		http.MethodPost, "/api/projects", strings.NewReader(`{"name":"Shop","template":"wordpress"}`),
	)
	response := httptest.NewRecorder()
	handler.HandleCollection(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", response.Code, response.Body)
	}
	var created serviceproject.Meta
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	inspect := httptest.NewRequest(
		http.MethodGet, "/api/projects/"+string(created.ID)+"/container", nil,
	)
	inspectResponse := httptest.NewRecorder()
	handler.HandleResource(inspectResponse, inspect)
	if inspectResponse.Code != http.StatusOK {
		t.Fatalf("container status = %d body = %s", inspectResponse.Code, inspectResponse.Body)
	}
	var info serviceproject.ContainerInspect
	if err := json.NewDecoder(inspectResponse.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.Template == nil || info.Template.Name != "wordpress" || info.Template.Status != "pending" {
		t.Fatalf("template status = %+v", info.Template)
	}
}
