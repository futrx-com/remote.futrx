package fileresources

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	serviceresources "github.com/futrx-com/remote.futrx.com/internal/service/resources"
)

func TestLoadReportsAbsentDocumentWithoutError(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	settings, found, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if found {
		t.Fatalf("found = true on an empty data dir, got %+v", settings)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	want := serviceresources.Settings{
		Defaults:             serviceresources.Limits{Memory: "2GiB", CPU: 2, Processes: 2000, Disk: "20GiB"},
		HostReserve:          serviceresources.Reserve{Memory: "768MiB", CPU: 0.5},
		MaxProjectOverride:   serviceresources.Limits{Memory: "3GiB", CPU: 4, Disk: "40GiB"},
		MaxRunningContainers: 3,
		UpdatedAt:            1700000000,
		UpdatedBy:            "admin@example.com",
	}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, found, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !found {
		t.Fatal("found = false after Save")
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}

	path := filepath.Join(dir, FileName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func TestSaveReplacesAnExistingDocument(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	first := serviceresources.Settings{Defaults: serviceresources.Limits{Memory: "2GiB", CPU: 2}}
	if err := store.Save(ctx, first); err != nil {
		t.Fatalf("Save: %v", err)
	}
	second := serviceresources.Settings{Defaults: serviceresources.Limits{Memory: "4GiB", CPU: 4}}
	if err := store.Save(ctx, second); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, _, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != second {
		t.Fatalf("Load = %+v, want %+v", got, second)
	}
}

func TestLoadRejectsCorruptDocument(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.Load(context.Background()); err == nil {
		t.Fatal("Load accepted a corrupt document")
	}
}
