package applications

import (
	"context"
	"reflect"
	"testing"
)

// fakeRegistry answers with UI-bearing applications for the ids it was given.
type fakeRegistry struct{ withUI, withBackend, withoutUI, builtin []string }

func (f *fakeRegistry) List() []Application { return nil }

func (f *fakeRegistry) Get(id string) (Application, bool) {
	for _, candidate := range f.withUI {
		if candidate == id {
			return Application{ID: id, Name: id, UI: &ApplicationUI{Entry: "scripts/main.js"}}, true
		}
	}
	for _, candidate := range f.withBackend {
		if candidate == id {
			return Application{
				ID: id, Name: id,
				UI:      &ApplicationUI{Entry: "scripts/main.js"},
				Backend: &ApplicationBackend{},
			}, true
		}
	}
	for _, candidate := range f.withoutUI {
		if candidate == id {
			return Application{ID: id, Name: id, Install: "infra/install.sh"}, true
		}
	}
	// An application the binary was built with, which is what a package uploaded
	// under the same id is up against.
	for _, candidate := range f.builtin {
		if candidate == id {
			return Application{ID: id, Name: id, Install: "infra/install.sh", Source: SourceBuiltin}, true
		}
	}
	return Application{}, false
}

func (f *fakeRegistry) UIAsset(string, string) ([]byte, bool) { return nil, false }

func instance(applicationID, projectID string, status InstanceStatus) Instance {
	scope := ScopeGlobal
	if projectID != "" {
		scope = ScopeProject
	}
	return Instance{
		ID:            applicationID + "-" + projectID,
		ApplicationID: applicationID,
		Scope:         scope,
		ProjectID:     projectID,
		Status:        status,
	}
}

func serviceWith(store *fakeStore, registry *fakeRegistry) *Service {
	return New(registry, store, nil, nil, nil)
}

func TestUIExtensionsReportsInstallScope(t *testing.T) {
	store := &fakeStore{
		global: []Instance{instance("global-app", "", StatusRunning)},
		byProject: map[string][]Instance{
			"p1": {instance("project-app", "p1", StatusRunning)},
			"p2": {instance("other-app", "p2", StatusRunning)},
		},
	}
	registry := &fakeRegistry{withUI: []string{"global-app", "project-app", "other-app"}}

	got, err := serviceWith(store, registry).UIExtensions(context.Background(), []string{"p1", "p2"})
	if err != nil {
		t.Fatalf("UIExtensions: %v", err)
	}

	want := map[string]UIExtension{
		"global-app":  {Global: true},
		"project-app": {ProjectIDs: []string{"p1"}},
		"other-app":   {ProjectIDs: []string{"p2"}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d extensions, want %d: %+v", len(got), len(want), got)
	}
	for _, ext := range got {
		expected, ok := want[ext.Application.ID]
		if !ok {
			t.Errorf("unexpected extension %s", ext.Application.ID)
			continue
		}
		if ext.Global != expected.Global {
			t.Errorf("%s global = %v, want %v", ext.Application.ID, ext.Global, expected.Global)
		}
		if !reflect.DeepEqual(ext.ProjectIDs, expected.ProjectIDs) {
			t.Errorf("%s projects = %v, want %v", ext.Application.ID, ext.ProjectIDs, expected.ProjectIDs)
		}
	}
}

// One application can be installed in several places at once; the union of those
// installs is what decides where it applies.
func TestUIExtensionsUnionsEveryInstallOfOneApplication(t *testing.T) {
	store := &fakeStore{
		global: []Instance{instance("app", "", StatusRunning)},
		byProject: map[string][]Instance{
			"p1": {instance("app", "p1", StatusRunning)},
			"p2": {instance("app", "p2", StatusRunning)},
		},
	}
	registry := &fakeRegistry{withUI: []string{"app"}}

	got, err := serviceWith(store, registry).UIExtensions(context.Background(), []string{"p1", "p2"})
	if err != nil {
		t.Fatalf("UIExtensions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want one entry per application, got %+v", got)
	}
	if !got[0].Global {
		t.Error("want global = true")
	}
	if !reflect.DeepEqual(got[0].ProjectIDs, []string{"p1", "p2"}) {
		t.Errorf("projects = %v, want [p1 p2]", got[0].ProjectIDs)
	}
}

// A project the caller cannot see is never passed in, so its installs must not
// reach them — this is the membership boundary, enforced by the caller.
func TestUIExtensionsIgnoresProjectsNotAskedFor(t *testing.T) {
	store := &fakeStore{
		byProject: map[string][]Instance{
			"mine":         {instance("app", "mine", StatusRunning)},
			"someone-else": {instance("secret-app", "someone-else", StatusRunning)},
		},
	}
	registry := &fakeRegistry{withUI: []string{"app", "secret-app"}}

	got, err := serviceWith(store, registry).UIExtensions(context.Background(), []string{"mine"})
	if err != nil {
		t.Fatalf("UIExtensions: %v", err)
	}
	if len(got) != 1 || got[0].Application.ID != "app" {
		t.Fatalf("want only the caller's own project extension, got %+v", got)
	}
}

func TestUIExtensionsSkipsStoppedAndUIlessApplications(t *testing.T) {
	store := &fakeStore{
		global: []Instance{
			instance("stopped-app", "", StatusStopped),
			instance("failed-app", "", StatusError),
			instance("no-ui-app", "", StatusRunning),
			instance("running-app", "", StatusRunning),
		},
	}
	registry := &fakeRegistry{
		withUI:    []string{"stopped-app", "failed-app", "running-app"},
		withoutUI: []string{"no-ui-app"},
	}

	got, err := serviceWith(store, registry).UIExtensions(context.Background(), nil)
	if err != nil {
		t.Fatalf("UIExtensions: %v", err)
	}
	if len(got) != 1 || got[0].Application.ID != "running-app" {
		t.Fatalf("want only the running UI application, got %+v", got)
	}
}
