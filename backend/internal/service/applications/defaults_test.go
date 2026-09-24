package applications

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

type applicationMapRegistry map[string]Application

func (r applicationMapRegistry) List() []Application {
	applications := make([]Application, 0, len(r))
	for _, application := range r {
		applications = append(applications, application)
	}
	return applications
}

func (r applicationMapRegistry) Get(id string) (Application, bool) {
	application, ok := r[id]
	return application, ok
}

func (applicationMapRegistry) UIAsset(string, string) ([]byte, bool) { return nil, false }

type fakeDefaultInstallationStore struct {
	installed map[string]bool
	listErr   error
	markErr   map[string]error
	listCalls int
	markCalls []string
}

func (s *fakeDefaultInstallationStore) ListDefaultInstallations(context.Context) ([]string, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	var ids []string
	for id, installed := range s.installed {
		if installed {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (s *fakeDefaultInstallationStore) MarkDefaultInstallation(_ context.Context, id string) error {
	s.markCalls = append(s.markCalls, id)
	if err := s.markErr[id]; err != nil {
		return err
	}
	if s.installed == nil {
		s.installed = map[string]bool{}
	}
	s.installed[id] = true
	return nil
}

func builtInDefault(id string) Application {
	return Application{
		ID:      id,
		Name:    id,
		Version: "1",
		Source:  SourceBuiltin,
		Scopes:  []Scope{ScopeGlobal},
	}
}

func defaultService(
	applications applicationMapRegistry,
	instances *fakeStore,
	defaults *fakeDefaultInstallationStore,
	ids ...string,
) *Service {
	return New(
		applications,
		instances,
		nil,
		nil,
		nil,
		WithDefaultApplications(defaults, ids...),
	)
}

func TestDefaultApplicationIDsAreEmptyUntilFileManagementLands(t *testing.T) {
	if ids := DefaultApplicationIDs(); len(ids) != 0 {
		t.Fatalf("production default applications = %v, want none", ids)
	}
}

func TestReconcileDefaultApplicationsInstallsAndMarksFreshDefault(t *testing.T) {
	instances := &fakeStore{}
	defaults := &fakeDefaultInstallationStore{}
	service := defaultService(
		applicationMapRegistry{"files": builtInDefault("files")},
		instances,
		defaults,
		"files",
	)

	if err := service.ReconcileDefaultApplications(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(instances.global) != 1 {
		t.Fatalf("global instances = %d, want 1", len(instances.global))
	}
	if got := instances.global[0]; got.ApplicationID != "files" || got.Status != StatusRunning {
		t.Errorf("installed instance = %+v, want running files", got)
	}
	if !defaults.installed["files"] {
		t.Error("successful default installation was not recorded")
	}
}

func TestReconcileDefaultApplicationsDoesNotRestoreAnUninstalledDefault(t *testing.T) {
	instances := &fakeStore{}
	defaults := &fakeDefaultInstallationStore{}
	service := defaultService(
		applicationMapRegistry{"files": builtInDefault("files")},
		instances,
		defaults,
		"files",
	)
	ctx := context.Background()

	if err := service.ReconcileDefaultApplications(ctx); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if err := service.Uninstall(ctx, instances.global[0].ID); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if err := service.ReconcileDefaultApplications(ctx); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if len(instances.global) != 0 {
		t.Fatalf("uninstalled default returned: %+v", instances.global)
	}
}

func TestReconcileDefaultApplicationsAdoptsExistingStateWithoutStartingIt(t *testing.T) {
	for _, status := range []InstanceStatus{StatusRunning, StatusStopped} {
		t.Run(string(status), func(t *testing.T) {
			instances := &fakeStore{global: []Instance{{
				ID: "existing", ApplicationID: "files", Scope: ScopeGlobal, Status: status,
			}}}
			defaults := &fakeDefaultInstallationStore{}
			service := defaultService(
				applicationMapRegistry{"files": builtInDefault("files")},
				instances,
				defaults,
				"files",
			)

			if err := service.ReconcileDefaultApplications(context.Background()); err != nil {
				t.Fatalf("reconcile: %v", err)
			}
			if len(instances.puts) != 0 {
				t.Fatalf("existing %s instance was rewritten: %+v", status, instances.puts)
			}
			if instances.global[0].Status != status {
				t.Errorf("status = %q, want preserved %q", instances.global[0].Status, status)
			}
			if !defaults.installed["files"] {
				t.Error("existing installation was not adopted")
			}
		})
	}
}

func TestReconcileDefaultApplicationsAddsOnlyNewlyIntroducedDefaults(t *testing.T) {
	instances := &fakeStore{}
	defaults := &fakeDefaultInstallationStore{installed: map[string]bool{"old": true}}
	service := defaultService(
		applicationMapRegistry{
			"old": builtInDefault("old"),
			"new": builtInDefault("new"),
		},
		instances,
		defaults,
		"old", "new",
	)

	if err := service.ReconcileDefaultApplications(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(instances.global) != 1 || instances.global[0].ApplicationID != "new" {
		t.Fatalf("installed instances = %+v, want only new", instances.global)
	}
}

func TestReconcileDefaultApplicationsRequiresBuiltInGlobalCatalogEntries(t *testing.T) {
	instances := &fakeStore{}
	defaults := &fakeDefaultInstallationStore{}
	service := defaultService(
		applicationMapRegistry{
			"uploaded": {
				ID: "uploaded", Name: "uploaded", Source: SourceUploaded, Scopes: []Scope{ScopeGlobal},
			},
			"project-only": {
				ID: "project-only", Name: "project-only", Source: SourceBuiltin, Scopes: []Scope{ScopeProject},
			},
			"good": builtInDefault("good"),
		},
		instances,
		defaults,
		"missing", "uploaded", "project-only", "good",
	)

	err := service.ReconcileDefaultApplications(context.Background())
	if !errors.Is(err, ErrUnknownApplication) {
		t.Errorf("reconcile error = %v, want ErrUnknownApplication", err)
	}
	if !errors.Is(err, ErrInvalidDefault) {
		t.Errorf("reconcile error = %v, want ErrInvalidDefault", err)
	}
	for _, id := range []string{"missing", "uploaded", "project-only"} {
		if defaults.installed[id] {
			t.Errorf("invalid default %q was marked installed", id)
		}
	}
	if len(instances.global) != 1 || instances.global[0].ApplicationID != "good" {
		t.Fatalf("valid later default was not installed after failures: %+v", instances.global)
	}
}

func TestReconcileDefaultApplicationsRetriesInstallFailureAndContinues(t *testing.T) {
	failing := builtInDefault("needs-input")
	failing.Env = []EnvVar{{Key: "TOKEN", Required: true}}
	instances := &fakeStore{}
	defaults := &fakeDefaultInstallationStore{}
	service := defaultService(
		applicationMapRegistry{
			"needs-input": failing,
			"good":        builtInDefault("good"),
		},
		instances,
		defaults,
		"needs-input", "good",
	)

	err := service.ReconcileDefaultApplications(context.Background())
	if !errors.Is(err, ErrRequiredEnv) {
		t.Fatalf("reconcile error = %v, want ErrRequiredEnv", err)
	}
	if defaults.installed["needs-input"] {
		t.Error("failed installation was marked complete")
	}
	if !defaults.installed["good"] {
		t.Error("later default was not reconciled after an install failure")
	}
	if len(instances.global) != 1 || instances.global[0].ApplicationID != "good" {
		t.Fatalf("global instances = %+v, want only good", instances.global)
	}
}

func TestReconcileDefaultApplicationsRecoversAfterMarkerWriteFailure(t *testing.T) {
	writeErr := errors.New("disk full")
	instances := &fakeStore{}
	defaults := &fakeDefaultInstallationStore{markErr: map[string]error{"files": writeErr}}
	service := defaultService(
		applicationMapRegistry{"files": builtInDefault("files")},
		instances,
		defaults,
		"files",
	)

	if err := service.ReconcileDefaultApplications(context.Background()); !errors.Is(err, writeErr) {
		t.Fatalf("first reconcile error = %v, want marker write failure", err)
	}
	if len(instances.global) != 1 {
		t.Fatalf("installation did not commit before marker failure: %+v", instances.global)
	}
	delete(defaults.markErr, "files")
	if err := service.ReconcileDefaultApplications(context.Background()); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if len(instances.global) != 1 {
		t.Fatalf("existing installation was duplicated: %+v", instances.global)
	}
	if !defaults.installed["files"] {
		t.Error("existing installation was not marked on retry")
	}
}

func TestReconcileDefaultApplicationsRetriesIncompleteInstances(t *testing.T) {
	for _, status := range []InstanceStatus{StatusError, StatusInstalling} {
		t.Run(string(status), func(t *testing.T) {
			instances := &fakeStore{global: []Instance{{
				ID: "partial", ApplicationID: "files", Scope: ScopeGlobal, Status: status,
			}}}
			defaults := &fakeDefaultInstallationStore{}
			service := defaultService(
				applicationMapRegistry{"files": builtInDefault("files")},
				instances,
				defaults,
				"files",
			)

			if err := service.ReconcileDefaultApplications(context.Background()); err != nil {
				t.Fatalf("reconcile: %v", err)
			}
			if !slices.Contains(instances.deleted, "partial") {
				t.Errorf("partial %s instance was not removed", status)
			}
			if len(instances.global) != 1 || instances.global[0].ID == "partial" || instances.global[0].Status != StatusRunning {
				t.Fatalf("replacement instance = %+v, want one new running instance", instances.global)
			}
		})
	}
}

func TestReconcileDefaultApplicationsIgnoresProjectInstallation(t *testing.T) {
	instances := &fakeStore{byProject: map[string][]Instance{
		"project-1": {{
			ID: "project-copy", ApplicationID: "files", Scope: ScopeProject,
			ProjectID: "project-1", Status: StatusRunning,
		}},
	}}
	defaults := &fakeDefaultInstallationStore{}
	service := defaultService(
		applicationMapRegistry{"files": builtInDefault("files")},
		instances,
		defaults,
		"files",
	)

	if err := service.ReconcileDefaultApplications(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(instances.global) != 1 || instances.global[0].ApplicationID != "files" {
		t.Fatalf("global default was not installed: %+v", instances.global)
	}
	if len(instances.byProject["project-1"]) != 1 {
		t.Fatal("project installation was changed")
	}
}

func TestReconcileDefaultApplicationsCannotProceedWithoutMarkers(t *testing.T) {
	readErr := errors.New("corrupt defaults file")
	instances := &fakeStore{}
	defaults := &fakeDefaultInstallationStore{listErr: readErr}
	service := defaultService(
		applicationMapRegistry{"files": builtInDefault("files")},
		instances,
		defaults,
		"files",
	)

	err := service.ReconcileDefaultApplications(context.Background())
	if !errors.Is(err, readErr) || !strings.Contains(err.Error(), "read default installations") {
		t.Fatalf("reconcile error = %v, want marker read failure", err)
	}
	if len(instances.global) != 0 {
		t.Fatalf("installed without knowing uninstall history: %+v", instances.global)
	}
}

func TestReconcileDefaultApplicationsDeduplicatesPolicyList(t *testing.T) {
	instances := &fakeStore{}
	defaults := &fakeDefaultInstallationStore{}
	service := defaultService(
		applicationMapRegistry{"files": builtInDefault("files")},
		instances,
		defaults,
		"files", "files",
	)

	if err := service.ReconcileDefaultApplications(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(instances.global) != 1 || len(defaults.markCalls) != 1 {
		t.Fatalf("duplicate policy entry installed/marked more than once: instances=%d marks=%v",
			len(instances.global), defaults.markCalls)
	}
}

func TestReconcileDefaultApplicationsWithEmptyPolicyDoesNotReadStore(t *testing.T) {
	defaults := &fakeDefaultInstallationStore{listErr: errors.New("must not be read")}
	service := New(applicationMapRegistry{}, &fakeStore{}, nil, nil, nil,
		WithDefaultApplications(defaults, DefaultApplicationIDs()...))

	if err := service.ReconcileDefaultApplications(context.Background()); err != nil {
		t.Fatalf("empty policy reconcile: %v", err)
	}
	if defaults.listCalls != 0 {
		t.Errorf("empty policy read the marker store %d times", defaults.listCalls)
	}
}

func TestReconcileDefaultApplicationsRequiresMarkerStoreForNonEmptyPolicy(t *testing.T) {
	service := New(
		applicationMapRegistry{"files": builtInDefault("files")},
		&fakeStore{},
		nil,
		nil,
		nil,
		WithDefaultApplications(nil, "files"),
	)

	err := service.ReconcileDefaultApplications(context.Background())
	if err == nil || !strings.Contains(err.Error(), "default installation store is unavailable") {
		t.Fatalf("reconcile error = %v, want unavailable marker store", err)
	}
}
