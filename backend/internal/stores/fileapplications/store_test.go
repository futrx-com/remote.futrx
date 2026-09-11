package fileapplications

import (
	"context"
	"testing"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func put(t *testing.T, store *Store, inst svc.Instance) {
	t.Helper()
	if err := store.Put(context.Background(), inst); err != nil {
		t.Fatal(err)
	}
}

func globalInstance(id string) svc.Instance {
	return svc.Instance{ID: id, ImageID: "db", Scope: svc.ScopeGlobal}
}

func projectInstance(id, projectID string) svc.Instance {
	return svc.Instance{ID: id, ImageID: "db", Scope: svc.ScopeProject, ProjectID: projectID}
}

// An instance is addressed by id alone — start, stop and uninstall all begin
// with one — so a lookup has to find it whichever file holds it, and has to say
// so when nothing does.
func TestGetFindsAnInstanceInAnyScope(t *testing.T) {
	store := newTestStore(t)
	put(t, store, globalInstance("g1"))
	put(t, store, projectInstance("p1a", "alpha"))
	put(t, store, projectInstance("p1b", "beta"))

	for id, wantProject := range map[string]string{"g1": "", "p1a": "alpha", "p1b": "beta"} {
		inst, ok, err := store.Get(context.Background(), id)
		if err != nil || !ok {
			t.Fatalf("get %s: ok = %v, err = %v", id, ok, err)
		}
		if inst.ProjectID != wantProject {
			t.Errorf("get %s: project = %q, want %q", id, inst.ProjectID, wantProject)
		}
	}
	// Repeated lookups go through the index; they must still answer the same.
	if _, ok, _ := store.Get(context.Background(), "p1b"); !ok {
		t.Error("a second lookup of the same instance missed it")
	}
	if _, ok, err := store.Get(context.Background(), "nope"); ok || err != nil {
		t.Errorf("get of an unknown id: ok = %v, err = %v", ok, err)
	}
}

// Deleting is what uninstall ends with. It has to remove the instance from the
// file that actually holds it and leave every other instance alone.
func TestDeleteRemovesOnlyTheNamedInstance(t *testing.T) {
	store := newTestStore(t)
	put(t, store, globalInstance("g1"))
	put(t, store, projectInstance("p1a", "alpha"))
	put(t, store, projectInstance("p1b", "beta"))

	if err := store.Delete(context.Background(), "p1a"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.Get(context.Background(), "p1a"); ok {
		t.Error("the deleted instance is still readable")
	}
	// Deleting an id that is gone is not an error: uninstall retries.
	if err := store.Delete(context.Background(), "p1a"); err != nil {
		t.Fatalf("second delete: %v", err)
	}

	all, err := store.ListAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("ListAll returned %d instances, want the 2 survivors: %+v", len(all), all)
	}
}
