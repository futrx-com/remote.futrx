package fileprojectshares

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceshare "github.com/futrx-com/remote.futrx.com/internal/service/share"
)

const projectID = serviceproject.ID("aaaa1111")

func TestListOnMissingProjectFileIsEmpty(t *testing.T) {
	store := newStore(t)
	shares, err := store.List(context.Background(), projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(shares) != 0 {
		t.Fatalf("List = %#v, want empty", shares)
	}
}

func TestUpdateRoundTripsRecordsAndKeepsTheFilePrivate(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	record := serviceshare.Share{
		ID:        "1f2e3d4c",
		TokenHash: strings.Repeat("a", 64),
		Port:      3000,
		Label:     "client demo",
		CreatedBy: "owner@example.com",
		CreatedAt: 1_700_000_000_000,
		ExpiresAt: 1_700_086_400_000,
	}

	saved, err := store.Update(ctx, projectID, func(current []serviceshare.Share) ([]serviceshare.Share, error) {
		return append(current, record), nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(saved) != 1 || saved[0] != record {
		t.Fatalf("Update returned %#v, want the appended record", saved)
	}

	reloaded, err := store.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(reloaded) != 1 || reloaded[0] != record {
		t.Fatalf("List = %#v, want the persisted record", reloaded)
	}

	path := filepath.Join(store.root, string(projectID)+".json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if runtimeSupportsFileModes() && info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, want 0600", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), record.TokenHash) {
		t.Fatal("the token digest is missing from the stored file")
	}
	if strings.Contains(string(raw), "\"token\"") {
		t.Fatal("the stored file has a plaintext token field")
	}
}

func TestUpdateErrorLeavesStoredRecordsUnchanged(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	record := serviceshare.Share{ID: "1f2e3d4c", TokenHash: "digest", Port: 3000, ExpiresAt: 1}

	if _, err := store.Update(ctx, projectID, func(current []serviceshare.Share) ([]serviceshare.Share, error) {
		return append(current, record), nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	sentinel := errors.New("boom")
	if _, err := store.Update(ctx, projectID, func([]serviceshare.Share) ([]serviceshare.Share, error) {
		return nil, sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("Update error = %v, want %v", err, sentinel)
	}

	shares, err := store.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(shares) != 1 || shares[0] != record {
		t.Fatalf("List after a failed update = %#v, want the original record", shares)
	}
}

func TestUpdateToEmptyRemovesTheFile(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if _, err := store.Update(ctx, projectID, func(current []serviceshare.Share) ([]serviceshare.Share, error) {
		return append(current, serviceshare.Share{ID: "1f2e3d4c", TokenHash: "digest", Port: 3000}), nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := store.Update(ctx, projectID, func([]serviceshare.Share) ([]serviceshare.Share, error) {
		return nil, nil
	}); err != nil {
		t.Fatalf("Update to empty: %v", err)
	}

	if _, err := os.Stat(filepath.Join(store.root, string(projectID)+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat after emptying = %v, want the file removed", err)
	}
}

func newStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store
}

// runtimeSupportsFileModes reports whether POSIX permission bits survive on
// this platform. Windows reports 0666 regardless of Chmod.
func runtimeSupportsFileModes() bool {
	return os.PathSeparator == '/'
}
