package project

import (
	"context"
	"errors"
	"testing"
)

type templateCatalogStub struct {
	known       map[string]bool
	defaultName string
	status      TemplateStatus
	statusCalls []string
	forgotten   []string
}

func newTemplateCatalogStub(names ...string) *templateCatalogStub {
	known := make(map[string]bool, len(names))
	for _, name := range names {
		known[name] = true
	}
	return &templateCatalogStub{
		known:       known,
		defaultName: DefaultTemplate,
		status:      TemplateStatus{Name: "wordpress", Title: "WordPress", Status: "running"},
	}
}

func (s *templateCatalogStub) Has(name string) bool { return s.known[name] }

func (s *templateCatalogStub) DefaultName() string { return s.defaultName }

func (s *templateCatalogStub) TemplateStatus(_ context.Context, container, template string) TemplateStatus {
	s.statusCalls = append(s.statusCalls, container+" "+template)
	return s.status
}

func (s *templateCatalogStub) ForgetTemplateState(containerName string) {
	s.forgotten = append(s.forgotten, containerName)
}

func TestCreateResolvesTheRequestedTemplate(t *testing.T) {
	tests := []struct {
		name       string
		catalog    ContainerTemplates
		requested  string
		want       string
		wantErr    error
		wantStored string
	}{
		{
			name:    "an empty request takes the catalog default",
			catalog: newTemplateCatalogStub("blank", "wordpress"),
			want:    DefaultTemplate,
		},
		{
			name:      "a known template is stored verbatim",
			catalog:   newTemplateCatalogStub("blank", "wordpress"),
			requested: "wordpress",
			want:      "wordpress",
		},
		{
			name:      "the request is normalized before lookup",
			catalog:   newTemplateCatalogStub("blank", "wordpress"),
			requested: "  WordPress ",
			want:      "wordpress",
		},
		{
			name:      "an unknown template is rejected",
			catalog:   newTemplateCatalogStub("blank", "wordpress"),
			requested: "cobol",
			wantErr:   ErrUnknownTemplate,
		},
		{
			name: "without a catalog only the default is accepted",
			want: DefaultTemplate,
		},
		{
			name:      "without a catalog a named template is rejected",
			requested: "wordpress",
			wantErr:   ErrUnknownTemplate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &startTestRepository{}
			service := New(repo, ContainerDependencies{Templates: tt.catalog}, nil, nil)

			meta, err := service.Create(
				context.Background(),
				CreateInput{Name: "My Project", Template: tt.requested},
				"",
			)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create() = %v", err)
			}
			if meta.Template != tt.want {
				t.Fatalf("Template = %q, want %q", meta.Template, tt.want)
			}
		})
	}
}

func TestCreateRejectsAnUnknownTemplateBeforeWritingAnything(t *testing.T) {
	repo := &startTestRepository{}
	catalog := newTemplateCatalogStub("blank")
	service := New(repo, ContainerDependencies{Templates: catalog}, nil, nil)

	if _, err := service.Create(
		context.Background(), CreateInput{Name: "P", Template: "cobol"}, "",
	); !errors.Is(err, ErrUnknownTemplate) {
		t.Fatalf("Create() error = %v, want %v", err, ErrUnknownTemplate)
	}
	if repo.meta.Name != "" {
		t.Fatalf("a rejected create still persisted %#v", repo.meta)
	}
}

func TestTemplateNameDefaultsForMetadataWrittenBeforeTemplatesExisted(t *testing.T) {
	tests := []struct {
		name string
		meta Meta
		want string
	}{
		{name: "absent field", meta: Meta{}, want: DefaultTemplate},
		{name: "explicit blank", meta: Meta{Template: "blank"}, want: "blank"},
		{name: "explicit stack", meta: Meta{Template: "wordpress"}, want: "wordpress"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.meta.TemplateName(); got != tt.want {
				t.Fatalf("TemplateName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpdateCannotChangeTheTemplate(t *testing.T) {
	repo := &startTestRepository{meta: Meta{
		ID: ID("abcd"), Name: "project", ContainerName: "project", Template: "wordpress",
	}}
	service := New(repo, ContainerDependencies{Templates: newTemplateCatalogStub("blank", "wordpress")}, nil, nil)

	renamed := "renamed"
	meta, err := service.Update(context.Background(), ID("abcd"), UpdateInput{Name: &renamed})
	if err != nil {
		t.Fatalf("Update() = %v", err)
	}
	if meta.Template != "wordpress" {
		t.Fatalf("Template = %q, want it unchanged", meta.Template)
	}
}

type templateInspector struct{ info ContainerInspect }

func (i templateInspector) Inspect(context.Context, string) (ContainerInspect, error) {
	return i.info, nil
}

func TestInspectContainerReportsTheTemplateStatus(t *testing.T) {
	repo := &startTestRepository{meta: Meta{
		ID: ID("abcd"), Name: "project", ContainerName: "project", Template: "wordpress",
	}}
	catalog := newTemplateCatalogStub("blank", "wordpress")
	service := New(repo, ContainerDependencies{
		Inspector: templateInspector{info: ContainerInspect{Name: "project"}},
		Templates: catalog,
	}, nil, nil)

	info, err := service.InspectContainer(context.Background(), ID("abcd"))
	if err != nil {
		t.Fatalf("InspectContainer() = %v", err)
	}
	if info.Template == nil || info.Template.Name != "wordpress" || info.Template.Status != "running" {
		t.Fatalf("Template = %+v", info.Template)
	}
	if len(catalog.statusCalls) != 1 || catalog.statusCalls[0] != "project wordpress" {
		t.Fatalf("statusCalls = %q", catalog.statusCalls)
	}
}

func TestDeleteForgetsCachedTemplateState(t *testing.T) {
	repo := &startTestRepository{meta: Meta{
		ID: ID("abcd"), Name: "project", ContainerName: "project", Template: "wordpress",
	}}
	catalog := newTemplateCatalogStub("blank", "wordpress")
	service := New(repo, ContainerDependencies{Templates: catalog}, nil, nil)

	if err := service.Delete(context.Background(), ID("abcd")); err != nil {
		t.Fatalf("Delete() = %v", err)
	}
	if len(catalog.forgotten) != 1 || catalog.forgotten[0] != "project" {
		t.Fatalf("forgotten = %q, want the deleted container", catalog.forgotten)
	}
}

func TestInspectContainerReportsTheDefaultTemplateWithoutACatalog(t *testing.T) {
	repo := &startTestRepository{meta: Meta{
		ID: ID("abcd"), Name: "project", ContainerName: "project",
	}}
	service := New(repo, ContainerDependencies{
		Inspector: templateInspector{info: ContainerInspect{Name: "project"}},
	}, nil, nil)

	info, err := service.InspectContainer(context.Background(), ID("abcd"))
	if err != nil {
		t.Fatalf("InspectContainer() = %v", err)
	}
	if info.Template == nil || info.Template.Name != DefaultTemplate {
		t.Fatalf("Template = %+v, want the default", info.Template)
	}
	if info.Template.Status != TemplateProvisioningNone {
		t.Fatalf("Status = %q, want %q", info.Template.Status, TemplateProvisioningNone)
	}
}
