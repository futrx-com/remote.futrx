package applications

import (
	"context"
	"testing"
)

func TestGlobalInstallPlacement(t *testing.T) {
	for _, tc := range []struct {
		name             string
		insideContainers bool
		wantGlobal       int
		wantPerProject   int
	}{
		{name: "inside every project container", insideContainers: true, wantPerProject: 1},
		{name: "dedicated container", insideContainers: false, wantGlobal: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := serviceApplicationAt("1")
			app.GloballyInstalledInsideContainers = tc.insideContainers
			store := &fakeStore{}
			projects := &staticProjects{projectIDs: []string{"p1", "p2"}}
			s := New(&singleApplicationRegistry{application: app}, store, &recordingInstaller{}, projects, &countingAllocator{})

			if _, err := s.Install(context.Background(), InstallRequest{ApplicationID: app.ID, Scope: ScopeGlobal}); err != nil {
				t.Fatal(err)
			}

			if got := len(store.global); got != tc.wantGlobal {
				t.Errorf("global instances = %d, want %d", got, tc.wantGlobal)
			}
			for _, projectID := range projects.projectIDs {
				if got := len(store.byProject[projectID]); got != tc.wantPerProject {
					t.Errorf("project %s instances = %d, want %d", projectID, got, tc.wantPerProject)
				}
			}
		})
	}
}
