package fileapplications

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
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

func TestDefaultInstallationsAreEmptyBeforeFirstSeed(t *testing.T) {
	store := newTestStore(t)
	ids, err := store.ListDefaultInstallations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("default installations = %v, want empty", ids)
	}
	if _, err := os.Stat(store.defaultsPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reading an empty default set created a file: %v", err)
	}
}

func TestDefaultInstallationMarkersPersistSortedAndIdempotent(t *testing.T) {
	dataDir := t.TempDir()
	store, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"zeta", "alpha", "zeta"} {
		if err := store.MarkDefaultInstallation(context.Background(), id); err != nil {
			t.Fatalf("mark %q: %v", id, err)
		}
	}

	reopened, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := reopened.ListDefaultInstallations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alpha", "zeta"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("default installations = %v, want %v", ids, want)
	}
	info, err := os.Stat(reopened.defaultsPath())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("defaults.json mode = %04o, want 0600", got)
	}
}

func TestDefaultInstallationMarkersDoNotLoseConcurrentWrites(t *testing.T) {
	store := newTestStore(t)
	const count = 32
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := store.MarkDefaultInstallation(context.Background(), id); err != nil {
				t.Errorf("mark %q: %v", id, err)
			}
		}(fmt.Sprintf("app-%02d", i))
	}
	wg.Wait()

	ids, err := store.ListDefaultInstallations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != count {
		t.Fatalf("default installations = %d, want %d: %v", len(ids), count, ids)
	}
	for i, id := range ids {
		if want := fmt.Sprintf("app-%02d", i); id != want {
			t.Errorf("default installation %d = %q, want %q", i, id, want)
		}
	}
}

func TestMalformedDefaultInstallationsFailClosed(t *testing.T) {
	store := newTestStore(t)
	if err := os.WriteFile(store.defaultsPath(), []byte(`{"installed":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListDefaultInstallations(context.Background()); err == nil {
		t.Fatal("malformed defaults.json was accepted")
	}
	if err := store.MarkDefaultInstallation(context.Background(), "files"); err == nil {
		t.Fatal("mark overwrote malformed defaults.json")
	}
}

func TestDefaultMarkersSurviveInstanceDeletionAndStayOutOfInstanceLists(t *testing.T) {
	store := newTestStore(t)
	if err := store.MarkDefaultInstallation(context.Background(), "files"); err != nil {
		t.Fatal(err)
	}
	put(t, store, globalInstance("instance-1"))
	if err := store.Delete(context.Background(), "instance-1"); err != nil {
		t.Fatal(err)
	}

	ids, err := store.ListDefaultInstallations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []string{"files"}) {
		t.Fatalf("default installations after delete = %v, want files", ids)
	}
	instances, err := store.ListAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 0 {
		t.Fatalf("defaults.json leaked into instance list: %+v", instances)
	}
	if _, err := os.Stat(filepath.Join(store.root, "defaults.json")); err != nil {
		t.Fatalf("defaults.json disappeared: %v", err)
	}
}

func TestDefaultInstallationRejectsEmptyID(t *testing.T) {
	store := newTestStore(t)
	if err := store.MarkDefaultInstallation(context.Background(), ""); err == nil {
		t.Fatal("empty default application id was accepted")
	}
}

func put(t *testing.T, store *Store, inst svc.Instance) {
	t.Helper()
	if err := store.Put(context.Background(), inst); err != nil {
		t.Fatal(err)
	}
}

func globalInstance(id string) svc.Instance {
	return svc.Instance{ID: id, ApplicationID: "db", Scope: svc.ScopeGlobal}
}

func projectInstance(id, projectID string) svc.Instance {
	return svc.Instance{ID: id, ApplicationID: "db", Scope: svc.ScopeProject, ProjectID: projectID}
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
