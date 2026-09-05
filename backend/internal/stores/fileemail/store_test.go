package fileemail

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	serviceemail "github.com/futrx-com/remote.futrx.com/internal/service/email"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	ctx := context.Background()

	creds, err := store.Credentials(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds != nil {
		t.Fatalf("Credentials before any write = %+v, want nil", creds)
	}

	want := serviceemail.Credentials{Address: "user@example.com", AppPassword: "abcdefghijklmnop"}
	if err := store.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Credentials(ctx)
	if err != nil {
		t.Fatalf("Credentials after write: %v", err)
	}
	if got == nil || *got != want {
		t.Fatalf("Credentials = %+v, want %+v", got, want)
	}

	info, err := os.Stat(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 0600", perm)
	}

	if err := store.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx); err != nil {
		t.Fatalf("Delete on already-missing file should be idempotent: %v", err)
	}

	creds, err = store.Credentials(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds != nil {
		t.Fatalf("Credentials after delete = %+v, want nil", creds)
	}
}
