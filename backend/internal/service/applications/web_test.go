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

func TestNamedWebTargetUsesProjectAndRejectsAmbiguity(t *testing.T) {
	ctx := context.Background()
	first := Instance{ID: "first", ApplicationID: "editor", ProjectID: "p1", Scope: ScopeProject, Status: StatusRunning}
	store := &fakeStore{byProject: map[string][]Instance{"p1": {first}}}
	registry := &singleApplicationRegistry{application: Application{ID: "editor", Web: &ApplicationWeb{Port: 8400, Subdomain: "code"}}}
	service := New(registry, store, nil, nil, nil)
	for _, tc := range []struct {
		project, label string
		want           bool
	}{
		{"p1", "code", true}, {"p2", "code", false}, {"p1", "wrong", false},
	} {
		_, ok, err := service.ProjectWebTargetBySubdomain(ctx, tc.project, tc.label)
		if err != nil || ok != tc.want {
			t.Fatal(tc, ok, err)
		}
	}
	second := first
	second.ID = "second"
	store.byProject["p1"] = []Instance{first, second}
	if _, ok, _ := service.ProjectWebTargetBySubdomain(ctx, "p1", "code"); ok {
		t.Fatal("ambiguous label accepted")
	}
	first.Status = StatusStopped
	store.byProject["p1"] = []Instance{first}
	if _, ok, _ := service.ProjectWebTargetBySubdomain(ctx, "p1", "code"); ok {
		t.Fatal("stopped app accepted")
	}
}
