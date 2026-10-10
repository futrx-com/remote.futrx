package applications

import (
	"context"
	"errors"
	"testing"
)

// projectFailingInstaller fails the install in one project and succeeds in
// the others.
type projectFailingInstaller struct {
	recordingInstaller
	failProjectID string
}

func (i *projectFailingInstaller) Install(ctx context.Context, spec InstallSpec) error {
	if spec.Instance.ProjectID == i.failProjectID {
		return errors.New("install failed")
	}
	return i.recordingInstaller.Install(ctx, spec)
}

func globalInstallService(insideContainers bool, installer Installer) (*Service, *fakeStore, *staticProjects) {
	app := serviceApplicationAt("1")
	app.GloballyInstalledInsideContainers = insideContainers
	store := &fakeStore{}
	projects := &staticProjects{projectIDs: []string{"p1", "p2"}}
	return New(&singleApplicationRegistry{application: app}, store, installer, projects, &countingAllocator{}), store, projects
}

func installGlobally(t *testing.T, s *Service) View {
	t.Helper()
	view, err := s.Install(context.Background(), InstallRequest{ApplicationID: "db", Scope: ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestGlobalInstallPlacement(t *testing.T) {
	for _, tc := range []struct {
		name             string
		insideContainers bool
		wantStatus       InstanceStatus
		wantPerProject   int
	}{
		{name: "inside every project container", insideContainers: true, wantStatus: StatusInProjects, wantPerProject: 1},
		{name: "dedicated container", insideContainers: false, wantStatus: StatusRunning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, store, projects := globalInstallService(tc.insideContainers, &recordingInstaller{})

			view := installGlobally(t, s)

			if view.Scope != ScopeGlobal || view.Status != tc.wantStatus {
				t.Errorf("returned %s instance with status %s, want global %s", view.Scope, view.Status, tc.wantStatus)
			}
			if len(store.global) != 1 || store.global[0].Status != tc.wantStatus {
				t.Errorf("global instances = %+v, want one with status %s", store.global, tc.wantStatus)
			}
			for _, projectID := range projects.projectIDs {
				if got := len(store.byProject[projectID]); got != tc.wantPerProject {
					t.Errorf("project %s instances = %d, want %d", projectID, got, tc.wantPerProject)
				}
			}
		})
	}
}

func TestGlobalInstallInsideContainersIsRefusedTwice(t *testing.T) {
	s, _, _ := globalInstallService(true, &recordingInstaller{})
	installGlobally(t, s)

	_, err := s.Install(context.Background(), InstallRequest{ApplicationID: "db", Scope: ScopeGlobal})
	if !errors.Is(err, ErrAlreadyInstalled) {
		t.Fatalf("second Install() error = %v, want ErrAlreadyInstalled", err)
	}
}

func TestGlobalInstallRecordsNothingWhenAProjectFails(t *testing.T) {
	s, store, _ := globalInstallService(true, &projectFailingInstaller{failProjectID: "p2"})

	_, err := s.Install(context.Background(), InstallRequest{ApplicationID: "db", Scope: ScopeGlobal})
	if err == nil {
		t.Fatal("Install() succeeded, want the p2 failure")
	}
	if len(store.global) != 0 {
		t.Fatalf("global instances = %+v, want none so the install can be retried", store.global)
	}
	if got := len(store.byProject["p1"]); got != 1 {
		t.Fatalf("project p1 instances = %d, want its copy kept", got)
	}
}

func TestNewProjectInheritsGlobalInstallInsideContainers(t *testing.T) {
	for _, tc := range []struct {
		name             string
		insideContainers bool
		want             int
	}{
		{name: "inside every project container", insideContainers: true, want: 1},
		{name: "dedicated container", insideContainers: false, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, store, _ := globalInstallService(tc.insideContainers, &recordingInstaller{})
			installGlobally(t, s)

			if err := s.InstallGlobalApplications(context.Background(), "p3"); err != nil {
				t.Fatal(err)
			}

			if got := len(store.byProject["p3"]); got != tc.want {
				t.Fatalf("new project instances = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNewProjectReportsAFailedGlobalApplication(t *testing.T) {
	s, _, _ := globalInstallService(true, &projectFailingInstaller{failProjectID: "p3"})
	installGlobally(t, s)

	if err := s.InstallGlobalApplications(context.Background(), "p3"); err == nil {
		t.Fatal("InstallGlobalApplications() succeeded, want the install failure")
	}
}

func TestUninstallingGlobalRecordUninstallsEveryProjectCopy(t *testing.T) {
	s, store, _ := globalInstallService(true, &recordingInstaller{})
	global := installGlobally(t, s)
	copies := []string{store.byProject["p1"][0].ID, store.byProject["p2"][0].ID}

	if err := s.Uninstall(context.Background(), global.ID); err != nil {
		t.Fatal(err)
	}

	deleted := map[string]bool{}
	for _, id := range store.deleted {
		deleted[id] = true
	}
	for _, id := range append(copies, global.ID) {
		if !deleted[id] {
			t.Errorf("instance %s was not uninstalled; deleted = %v", id, store.deleted)
		}
	}
}

func TestGlobalRecordCannotBeStartedOrStopped(t *testing.T) {
	s, _, _ := globalInstallService(true, &recordingInstaller{})
	global := installGlobally(t, s)

	if _, err := s.Stop(context.Background(), global.ID); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Stop() error = %v, want ErrInvalidState", err)
	}
}
