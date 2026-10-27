package fileusersettings

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceusersettings "github.com/futrx-com/remote.futrx.com/internal/service/usersettings"
)

func TestStoreRoundTripUsesHashedFilename(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	settings := serviceusersettings.DefaultSettings()
	settings.Appearance.Theme = serviceusersettings.ThemeLight
	settings.Chat.AccountID = "codex-work"
	settings.UpdatedAt = 123

	key := serviceusersettings.Key("sub:google-user-123")
	if _, err := store.Save(context.Background(), key, settings); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Appearance.Theme != serviceusersettings.ThemeLight ||
		got.Chat.AccountID != "codex-work" || got.UpdatedAt != 123 {
		t.Fatalf("unexpected settings: %+v", got)
	}

	entries, err := os.ReadDir(store.root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one settings file, got %d", len(entries))
	}
	name := entries[0].Name()
	if !strings.HasPrefix(name, "sha256-") || !strings.HasSuffix(name, ".json") {
		t.Fatalf("settings file is not hashed: %s", name)
	}
	if strings.Contains(name, "google-user-123") {
		t.Fatalf("settings filename leaked the identity: %s", name)
	}
	if _, err := os.Stat(filepath.Join(store.root, name)); err != nil {
		t.Fatal(err)
	}
}

func TestStoreMissingSettings(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Get(context.Background(), serviceusersettings.Key("sub:missing"))
	if err != serviceusersettings.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
