package applications

import (
	"context"
	"errors"
	"testing"
)

type projectCleanupInstaller struct {
	Installer
	uninstallErr error
	uninstalled  []InstallSpec
}

func (i *projectCleanupInstaller) Uninstall(_ context.Context, spec InstallSpec) error {
	i.uninstalled = append(i.uninstalled, spec)
	return i.uninstallErr
}

func projectCleanupApplication() Application {
	return Application{
		ID:      "demo",
		Name:    "Demo",
		Install: "infra/install.sh",
		Scopes:  []Scope{ScopeProject},
	}
}

func TestDeleteProjectApplicationsUninstallsOnlyTargetProject(t *testing.T) {
	target := instance("demo", "p1", StatusRunning)
	other := instance("demo", "p2", StatusRunning)
	global := instance("demo", "", StatusRunning)

	// Include malformed entries to verify the scope and project checks.
	wrongProject := instance("demo", "p2", StatusRunning)
	wrongProject.ID = "wrong-project"
	globalInProject := global
	globalInProject.ID = "global-in-project"

	store := &fakeStore{
		global: []Instance{global},
		byProject: map[string][]Instance{
			"p1": {target, wrongProject, globalInProject},
			"p2": {other},
		},
	}

	installer := &projectCleanupInstaller{}
	service := New(
		&singleApplicationRegistry{application: projectCleanupApplication()},
		store,
		installer,
		nil,
		nil,
	)

	if err := service.DeleteProjectApplications(context.Background(), "p1"); err != nil {
		t.Fatalf("DeleteProjectApplications: %v", err)
	}

	if len(installer.uninstalled) != 1 {
		t.Fatalf("uninstall calls = %d, want 1", len(installer.uninstalled))
	}
	if got := installer.uninstalled[0].Instance.ID; got != target.ID {
		t.Fatalf("uninstalled instance = %q, want %q", got, target.ID)
	}

	if len(store.deleted) != 1 || store.deleted[0] != target.ID {
		t.Fatalf("deleted instances = %v, want [%s]", store.deleted, target.ID)
	}

	if _, found, _ := store.Get(context.Background(), other.ID); !found {
		t.Fatal("application in another project was deleted")
	}
	if _, found, _ := store.Get(context.Background(), global.ID); !found {
		t.Fatal("global application was deleted")
	}
}

func TestDeleteProjectApplicationsStopsWhenUninstallFails(t *testing.T) {
	target := instance("demo", "p1", StatusRunning)
	wantErr := errors.New("container unavailable")

	store := &fakeStore{
		byProject: map[string][]Instance{"p1": {target}},
	}
	installer := &projectCleanupInstaller{uninstallErr: wantErr}

	service := New(
		&singleApplicationRegistry{application: projectCleanupApplication()},
		store,
		installer,
		nil,
		nil,
	)

	err := service.DeleteProjectApplications(context.Background(), "p1")

	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped error %v", err, wantErr)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("deleted instances = %v, want none after uninstall failure", store.deleted)
	}
}
