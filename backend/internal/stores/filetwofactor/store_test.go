package filetwofactor

import (
	"context"
	"os"
	"strings"
	"testing"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

func TestStoreGetMissingReturnsNoErrorNoRecord(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	record, err := store.Get(context.Background(), "nobody@example.com")
	if err != nil {
		t.Fatalf("Get on missing record returned an error: %v", err)
	}
	if record != nil {
		t.Fatalf("Get on missing record = %+v, want nil", record)
	}
}

func TestStoreSaveGetRoundTripAndHashedFilename(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	record := serviceauth.TwoFactorRecord{
		Secret:             []byte("super-secret"),
		RecoveryCodeHashes: []string{"hash-a", "hash-b"},
		EnabledAt:          1000,
	}
	if err := store.Save(context.Background(), "User@Example.com", record); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get(context.Background(), "user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || string(got.Secret) != "super-secret" || len(got.RecoveryCodeHashes) != 2 || got.EnabledAt != 1000 {
		t.Fatalf("round trip mismatch: %+v", got)
	}

	entries, err := os.ReadDir(store.root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one record file, got %d", len(entries))
	}
	if strings.Contains(entries[0].Name(), "user") {
		t.Fatalf("filename leaked the identity: %s", entries[0].Name())
	}
}

func TestStoreSaveOverwritesAtomically(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	email := "user@example.com"
	if err := store.Save(context.Background(), email, serviceauth.TwoFactorRecord{EnabledAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), email, serviceauth.TwoFactorRecord{EnabledAt: 2}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	if got.EnabledAt != 2 {
		t.Fatalf("EnabledAt = %d, want 2 (overwrite did not take)", got.EnabledAt)
	}

	entries, err := os.ReadDir(store.root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one file after overwrite, got %d (temp file leaked?)", len(entries))
	}
}

func TestStoreDelete(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	email := "user@example.com"
	if err := store.Save(context.Background(), email, serviceauth.TwoFactorRecord{EnabledAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), email); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("Get after Delete = %+v, want nil", got)
	}
	// Deleting again is a no-op, not an error.
	if err := store.Delete(context.Background(), email); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}
