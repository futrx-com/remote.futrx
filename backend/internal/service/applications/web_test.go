package applications

import (
	"context"
	"testing"
)

func TestWebPortRequiresRunningDeclaredProjectInstallation(t *testing.T) {
	store := &fakeStore{byProject: map[string][]Instance{
		"p1": {{ID: "editor-1", ApplicationID: "editor", ProjectID: "p1", Scope: ScopeProject, Status: StatusStopped}},
		"p2": {{ID: "editor-2", ApplicationID: "editor", ProjectID: "p2", Scope: ScopeProject, Status: StatusRunning}},
	}}
	registry := &singleApplicationRegistry{application: Application{ID: "editor", Web: &ApplicationWeb{Port: 8400}}}
	service := New(registry, store, nil, nil, nil)
	for _, tc := range []struct {
		projectID, applicationID string
		wantPort                 int
		wantOK                   bool
	}{
		{"p1", "editor", 0, false},
		{"p2", "editor", 8400, true},
		{"p2", "missing", 0, false},
		{"p3", "editor", 0, false},
	} {
		port, ok, err := service.WebPort(context.Background(), tc.projectID, tc.applicationID)
		if err != nil || port != tc.wantPort || ok != tc.wantOK {
			t.Errorf("WebPort(%q, %q) = %d, %v, %v", tc.projectID, tc.applicationID, port, ok, err)
		}
	}
}
