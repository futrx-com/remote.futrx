package fileprojectaccess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

const project = serviceproject.ID("abcd1234")

func newStore(t *testing.T, dataDir string) *Store {
	t.Helper()
	store, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func mustList(t *testing.T, store *Store, id serviceproject.ID) []string {
	t.Helper()
	members, err := store.List(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return members
}

func TestAddNormalizesAndDeduplicatesEmails(t *testing.T) {
	store := newStore(t, t.TempDir())
	ctx := context.Background()

	for _, email := range []string{" Alice@Example.com ", "alice@example.com", "bob@example.com"} {
		if err := store.Add(ctx, project, email); err != nil {
			t.Fatalf("Add(%q) error = %v", email, err)
		}
	}

	if got, want := mustList(t, store, project), []string{"alice@example.com", "bob@example.com"}; !slices.Equal(got, want) {
		t.Fatalf("List() = %q, want %q", got, want)
	}
	if ok, err := store.Has(ctx, project, "ALICE@example.COM"); err != nil || !ok {
		t.Fatalf("Has(ALICE@example.COM) = %v, %v; want true", ok, err)
	}
}

func TestEmptyEmailsAreRejectedOrIgnored(t *testing.T) {
	store := newStore(t, t.TempDir())
	ctx := context.Background()

	if err := store.Add(ctx, project, "   "); err == nil {
		t.Fatal("Add(blank) error = nil, want an error")
	}
	if ok, err := store.Has(ctx, project, ""); err != nil || ok {
		t.Fatalf("Has(blank) = %v, %v; want false, nil", ok, err)
	}
	if err := store.Remove(ctx, project, " "); err != nil {
		t.Fatalf("Remove(blank) error = %v, want nil", err)
	}
}

func TestRemovingTheLastMemberDeletesTheFile(t *testing.T) {
	dataDir := t.TempDir()
	store := newStore(t, dataDir)
	ctx := context.Background()
	file := filepath.Join(dataDir, "projectaccess", string(project)+".json")

	if err := store.Add(ctx, project, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("access file after Add: %v", err)
	}
	if err := store.Remove(ctx, project, "Alice@Example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("access file after removing the last member: stat error = %v, want not exist", err)
	}
	if got := mustList(t, store, project); len(got) != 0 {
		t.Fatalf("List() = %q, want empty", got)
	}
	if err := store.Remove(ctx, project, "nobody@example.com"); err != nil {
		t.Fatalf("Remove(unknown) error = %v, want nil", err)
	}
}

func TestSetReplacesTheListAndSurvivesANewStore(t *testing.T) {
	dataDir := t.TempDir()
	ctx := context.Background()
	store := newStore(t, dataDir)
	if err := store.Add(ctx, project, "old@example.com"); err != nil {
		t.Fatal(err)
	}

	if err := store.Set(ctx, project, []string{"Carol@example.com", "", "bob@example.com", "carol@example.com"}); err != nil {
		t.Fatal(err)
	}

	reopened := newStore(t, dataDir)
	if got, want := mustList(t, reopened, project), []string{"bob@example.com", "carol@example.com"}; !slices.Equal(got, want) {
		t.Fatalf("List() after reopening = %q, want %q", got, want)
	}
}

func TestProjectsAreIndependentAndDeleteAllIsIdempotent(t *testing.T) {
	store := newStore(t, t.TempDir())
	ctx := context.Background()
	other := serviceproject.ID("ffff0000")
	if err := store.Add(ctx, project, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(ctx, other, "bob@example.com"); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err := store.DeleteAll(ctx, project); err != nil {
			t.Fatalf("DeleteAll() error = %v", err)
		}
	}

	if got := mustList(t, store, project); len(got) != 0 {
		t.Fatalf("List(deleted project) = %q, want empty", got)
	}
	if got := mustList(t, store, other); !slices.Equal(got, []string{"bob@example.com"}) {
		t.Fatalf("List(other project) = %q, want it untouched", got)
	}
}

func TestACorruptFileIsReportedNotTreatedAsEmpty(t *testing.T) {
	dataDir := t.TempDir()
	store := newStore(t, dataDir)
	file := filepath.Join(dataDir, "projectaccess", string(project)+".json")
	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.List(context.Background(), project); err == nil {
		t.Fatal("List() error = nil, want a parse error")
	}
	if err := store.Add(context.Background(), project, "alice@example.com"); err == nil {
		t.Fatal("Add() error = nil, want the parse error rather than overwriting the file")
	}
}

func TestConcurrentAddsKeepEveryMember(t *testing.T) {
	store := newStore(t, t.TempDir())
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.Add(context.Background(), project, fmt.Sprintf("user%02d@example.com", i)); err != nil {
				t.Errorf("Add() error = %v", err)
			}
		}()
	}
	wg.Wait()

	if got := mustList(t, store, project); len(got) != 20 {
		t.Fatalf("List() has %d members, want 20: %q", len(got), got)
	}
}
