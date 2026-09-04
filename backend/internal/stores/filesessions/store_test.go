package filesessions

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

	record := serviceauth.SessionRegistryRecord{
		Preferences:     serviceauth.SecurityPreferences{SingleSessionEnabled: true, HistoryEnabled: true},
		ActiveSessionID: "sid-1",
		History: serviceauth.SessionHistory{Entries: []serviceauth.SessionRecord{
			{SID: "sid-1", Method: serviceauth.SignInMethodPassword, IssuedAt: 1000},
		}},
	}
	if err := store.Save(context.Background(), "User@Example.com", record); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get(context.Background(), "user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !got.Preferences.SingleSessionEnabled || got.ActiveSessionID != "sid-1" || len(got.History.Entries) != 1 {
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
	if err := store.Save(context.Background(), email, serviceauth.SessionRegistryRecord{ActiveSessionID: "sid-1"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), email, serviceauth.SessionRegistryRecord{ActiveSessionID: "sid-2"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveSessionID != "sid-2" {
		t.Fatalf("ActiveSessionID = %q, want sid-2 (overwrite did not take)", got.ActiveSessionID)
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
	if err := store.Save(context.Background(), email, serviceauth.SessionRegistryRecord{ActiveSessionID: "sid-1"}); err != nil {
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
	if err := store.Delete(context.Background(), email); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}
