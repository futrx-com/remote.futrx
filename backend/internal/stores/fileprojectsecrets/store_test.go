package fileprojectsecrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func mustList(t *testing.T, store *Store, id serviceproject.ID) []serviceproject.Secret {
	t.Helper()
	secrets, err := store.List(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return secrets
}

func TestSetStoresAndOverwritesValuesSortedByKey(t *testing.T) {
	store := newStore(t, t.TempDir())
	ctx := context.Background()

	for _, secret := range [][2]string{{"TOKEN", "first"}, {"API_URL", "https://example.com"}, {"TOKEN", "second"}} {
		saved, err := store.Set(ctx, project, secret[0], secret[1])
		if err != nil {
			t.Fatalf("Set(%s) error = %v", secret[0], err)
		}
		if saved.Key != secret[0] || saved.Value != secret[1] || saved.UpdatedAt <= 0 {
			t.Fatalf("Set(%s) = %#v", secret[0], saved)
		}
	}

	got := mustList(t, store, project)
	if len(got) != 2 || got[0].Key != "API_URL" || got[1].Key != "TOKEN" || got[1].Value != "second" {
		t.Fatalf("List() = %#v, want API_URL then TOKEN=second", got)
	}
}

func TestMultilineValuesRoundTripUnchanged(t *testing.T) {
	dataDir := t.TempDir()
	value := "-----BEGIN KEY-----\nline one\nline two\n-----END KEY-----\n"
	if _, err := newStore(t, dataDir).Set(context.Background(), project, "SSH_KEY", value); err != nil {
		t.Fatal(err)
	}

	got := mustList(t, newStore(t, dataDir), project)
	if len(got) != 1 || got[0].Value != value {
		t.Fatalf("List() after reopening = %#v, want the value byte for byte", got)
	}
}

func TestDeletingTheLastSecretDeletesTheFile(t *testing.T) {
	dataDir := t.TempDir()
	store := newStore(t, dataDir)
	ctx := context.Background()
	file := filepath.Join(dataDir, "projectsecrets", string(project)+".json")

	if _, err := store.Set(ctx, project, "TOKEN", "value"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, project, "MISSING"); err != nil {
		t.Fatalf("Delete(unknown) error = %v, want nil", err)
	}
	if err := store.Delete(ctx, project, "TOKEN"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("secrets file after deleting the last key: stat error = %v, want not exist", err)
	}
	if got := mustList(t, store, project); len(got) != 0 {
		t.Fatalf("List() = %#v, want empty", got)
	}
}

func TestSecretsFileIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes")
	}
	dataDir := t.TempDir()
	if _, err := newStore(t, dataDir).Set(context.Background(), project, "TOKEN", "value"); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{
		filepath.Join(dataDir, "projectsecrets"):                          0o700,
		filepath.Join(dataDir, "projectsecrets", string(project)+".json"): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode = %o, want %o", path, got, want)
		}
	}
}

func TestProjectsAreIndependentAndDeleteAllIsIdempotent(t *testing.T) {
	store := newStore(t, t.TempDir())
	ctx := context.Background()
	other := serviceproject.ID("ffff0000")
	if _, err := store.Set(ctx, project, "TOKEN", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Set(ctx, other, "TOKEN", "b"); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err := store.DeleteAll(ctx, project); err != nil {
			t.Fatalf("DeleteAll() error = %v", err)
		}
	}

	if got := mustList(t, store, project); len(got) != 0 {
		t.Fatalf("List(deleted project) = %#v, want empty", got)
	}
	if got := mustList(t, store, other); len(got) != 1 || got[0].Value != "b" {
		t.Fatalf("List(other project) = %#v, want it untouched", got)
	}
}

func TestACorruptFileIsReportedNotOverwritten(t *testing.T) {
	dataDir := t.TempDir()
	store := newStore(t, dataDir)
	file := filepath.Join(dataDir, "projectsecrets", string(project)+".json")
	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.List(context.Background(), project); err == nil {
		t.Fatal("List() error = nil, want a parse error")
	}
	if _, err := store.Set(context.Background(), project, "TOKEN", "value"); err == nil {
		t.Fatal("Set() error = nil, want the parse error rather than replacing every secret")
	}
	if raw, _ := os.ReadFile(file); string(raw) != "{not json" {
		t.Fatalf("secrets file = %q, want it left as it was", raw)
	}
}

func TestConcurrentSetsKeepEveryKey(t *testing.T) {
	store := newStore(t, t.TempDir())
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.Set(context.Background(), project, fmt.Sprintf("KEY_%02d", i), "value"); err != nil {
				t.Errorf("Set() error = %v", err)
			}
		}()
	}
	wg.Wait()

	if got := mustList(t, store, project); len(got) != 20 {
		t.Fatalf("List() has %d keys, want 20", len(got))
	}
}
