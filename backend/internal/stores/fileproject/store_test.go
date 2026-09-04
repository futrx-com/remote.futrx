package fileproject

import (
	"context"
	"errors"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

func TestStoreCreatesUniqueSlugsAndLooksUpBySlug(t *testing.T) {
	store, err := NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.Create(context.Background(), serviceproject.Meta{
		Name:   "My Project",
		Slug:   serviceproject.Slugify("My Project"),
		Status: serviceproject.StatusProvisioning,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(context.Background(), serviceproject.Meta{
		Name:   "My-Project",
		Slug:   serviceproject.Slugify("My-Project"),
		Status: serviceproject.StatusProvisioning,
	})
	if err != nil {
		t.Fatal(err)
	}

	if first.Slug != "my-project" {
		t.Fatalf("first slug = %q", first.Slug)
	}
	if second.Slug != "my-project-2" {
		t.Fatalf("second slug = %q", second.Slug)
	}
	if first.Cwd == "" || second.Cwd == "" || first.Cwd == second.Cwd {
		t.Fatalf("unexpected cwd values: first=%q second=%q", first.Cwd, second.Cwd)
	}

	got, err := store.GetBySlug(context.Background(), first.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != first.ID {
		t.Fatalf("lookup id = %q, want %q", got.ID, first.ID)
	}

	if err := store.Delete(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetBySlug(context.Background(), first.Slug); err == nil {
		t.Fatal("expected deleted slug lookup to fail")
	}
}

func TestStoreRejectsDuplicateProjectNames(t *testing.T) {
	store, err := NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.Create(context.Background(), serviceproject.Meta{Name: "My Project"})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"My Project", "  mY pRoJeCt  "} {
		t.Run(name, func(t *testing.T) {
			_, err := store.Create(context.Background(), serviceproject.Meta{Name: name})
			if !errors.Is(err, serviceproject.ErrNameAlreadyExists) {
				t.Fatalf("Create(%q) error = %v, want %v", name, err, serviceproject.ErrNameAlreadyExists)
			}
		})
	}

	projects, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].ID != first.ID {
		t.Fatalf("projects = %#v, want only %q", projects, first.ID)
	}
}

func TestStoreRejectsRenamingProjectToDuplicateName(t *testing.T) {
	store, err := NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Create(context.Background(), serviceproject.Meta{Name: "First Project"}); err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(context.Background(), serviceproject.Meta{Name: "Second Project"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Update(context.Background(), second.ID, func(project *serviceproject.Meta) {
		project.Name = " first PROJECT "
	})
	if !errors.Is(err, serviceproject.ErrNameAlreadyExists) {
		t.Fatalf("Update() error = %v, want %v", err, serviceproject.ErrNameAlreadyExists)
	}

	unchanged, err := store.Get(context.Background(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Name != "Second Project" {
		t.Fatalf("name = %q, want %q", unchanged.Name, "Second Project")
	}
}

func TestStoreRejectsConcurrentDuplicateProjectNames(t *testing.T) {
	store, err := NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const attempts = 8
	results := make(chan error, attempts)
	for range attempts {
		go func() {
			_, err := store.Create(context.Background(), serviceproject.Meta{Name: "Concurrent Project"})
			results <- err
		}()
	}

	created := 0
	duplicates := 0
	for range attempts {
		err := <-results
		switch {
		case err == nil:
			created++
		case errors.Is(err, serviceproject.ErrNameAlreadyExists):
			duplicates++
		default:
			t.Fatalf("Create() error = %v", err)
		}
	}
	if created != 1 || duplicates != attempts-1 {
		t.Fatalf("created = %d, duplicates = %d; want 1 and %d", created, duplicates, attempts-1)
	}
}

func TestStoreAllowsReusingProjectNameAfterDelete(t *testing.T) {
	store, err := NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	project, err := store.Create(context.Background(), serviceproject.Meta{Name: "Reusable Project"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), project.ID); err != nil {
		t.Fatal(err)
	}

	reused, err := store.Create(context.Background(), serviceproject.Meta{Name: "  reusable PROJECT  "})
	if err != nil {
		t.Fatalf("Create() after delete: %v", err)
	}
	if reused.Slug != project.Slug {
		t.Fatalf("reused slug = %q, want %q", reused.Slug, project.Slug)
	}
}

func TestStoreRecognizesEveryLegacyDuplicateName(t *testing.T) {
	dataDir := t.TempDir()
	workspaceRoot := t.TempDir()
	store, err := NewWithWorkspaceRoot(dataDir, workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.Create(context.Background(), serviceproject.Meta{Name: "Legacy Project"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(context.Background(), serviceproject.Meta{Name: "Different Project"})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate metadata written by an older release that allowed duplicate
	// display names, then reload the indexes from disk.
	second.Name = " legacy PROJECT "
	if err := store.writeMeta(second); err != nil {
		t.Fatal(err)
	}
	store, err = NewWithWorkspaceRoot(dataDir, workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}

	assertDuplicate := func() {
		t.Helper()
		_, err := store.Create(context.Background(), serviceproject.Meta{Name: "Legacy Project"})
		if !errors.Is(err, serviceproject.ErrNameAlreadyExists) {
			t.Fatalf("Create() error = %v, want %v", err, serviceproject.ErrNameAlreadyExists)
		}
	}
	assertDuplicate()
	if err := store.Delete(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	assertDuplicate()
	if err := store.Delete(context.Background(), second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), serviceproject.Meta{Name: "Legacy Project"}); err != nil {
		t.Fatalf("Create() after deleting every duplicate: %v", err)
	}
}
