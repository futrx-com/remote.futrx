package fileaccountcredentials

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return store, dir
}

func credential(version int64) serviceauth.AccountCredential {
	return serviceauth.AccountCredential{
		PasswordHash:       "$argon2id$hash",
		MustChangePassword: true,
		Version:            version,
		UpdatedAt:          time.Unix(1700000000, 0).UTC(),
	}
}

func TestGetAbsentReturnsNilWithoutCreatingTheFile(t *testing.T) {
	store, dir := newStore(t)
	got, err := store.Get(context.Background(), "nobody@example.com")
	if err != nil || got != nil {
		t.Fatalf("Get = %+v, %v; want nil, nil", got, err)
	}
	list, err := store.List(context.Background())
	if err != nil || len(list) != 0 {
		t.Fatalf("List = %+v, %v; want empty", list, err)
	}
	if _, err := os.Stat(filepath.Join(dir, fileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reading created the file: %v", err)
	}
}

func TestSaveGetListRoundTripNormalizesEmail(t *testing.T) {
	store, _ := newStore(t)
	ctx := context.Background()
	if err := store.Save(ctx, "  User@Example.COM ", credential(1)); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, "other@example.com", credential(7)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, "user@example.com")
	if err != nil || got == nil || *got != credential(1) {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	list, err := store.List(ctx)
	if err != nil || len(list) != 2 || list["user@example.com"] != credential(1) || list["other@example.com"] != credential(7) {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if again, _ := store.Get(ctx, " USER@example.com"); again == nil {
		t.Fatal("lookup is not case/space insensitive")
	}
}

func TestSaveOverwritesAndSurvivesReopen(t *testing.T) {
	store, dir := newStore(t)
	ctx := context.Background()
	if err := store.Save(ctx, "a@example.com", credential(1)); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, "a@example.com", credential(2)); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(ctx, "a@example.com")
	if err != nil || got == nil || got.Version != 2 {
		t.Fatalf("after reopen = %+v, %v", got, err)
	}
}

func TestDeleteIsIdempotent(t *testing.T) {
	store, dir := newStore(t)
	ctx := context.Background()
	if err := store.Delete(ctx, "a@example.com"); err != nil {
		t.Fatalf("delete from absent file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, fileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("delete of nothing created the file: %v", err)
	}
	if err := store.Save(ctx, "a@example.com", credential(1)); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, "b@example.com", credential(1)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := store.Delete(ctx, "A@example.com"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	}
	if got, _ := store.Get(ctx, "a@example.com"); got != nil {
		t.Fatalf("deleted credential still present: %+v", got)
	}
	if got, _ := store.Get(ctx, "b@example.com"); got == nil {
		t.Fatal("delete removed a neighbouring credential")
	}
}

func TestFileModeIs0600AndNoTempFilesLeak(t *testing.T) {
	store, dir := newStore(t)
	ctx := context.Background()
	for i := range 3 {
		if err := store.Save(ctx, "a@example.com", credential(int64(i+1))); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Delete(ctx, "a@example.com"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != fileName {
		t.Fatalf("data dir = %v, want only %s", entries, fileName)
	}
}

func TestSaveRejectsIncompleteCredentials(t *testing.T) {
	store, dir := newStore(t)
	ctx := context.Background()
	bad := map[string]struct {
		email string
		cred  serviceauth.AccountCredential
	}{
		"empty email": {"  ", credential(1)},
		"empty hash":  {"a@example.com", serviceauth.AccountCredential{Version: 1}},
		"no version":  {"a@example.com", serviceauth.AccountCredential{PasswordHash: "h"}},
	}
	for name, tc := range bad {
		if err := store.Save(ctx, tc.email, tc.cred); err == nil {
			t.Errorf("%s: Save succeeded", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, fileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected saves created the file: %v", err)
	}
}

func TestCorruptFileFailsEveryOperationAndIsNeverOverwritten(t *testing.T) {
	store, dir := newStore(t)
	ctx := context.Background()
	path := filepath.Join(dir, fileName)
	const garbage = "{not json"
	if err := os.WriteFile(path, []byte(garbage), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "a@example.com"); err == nil {
		t.Error("Get on a corrupt file succeeded")
	}
	if _, err := store.List(ctx); err == nil {
		t.Error("List on a corrupt file succeeded")
	}
	if err := store.Save(ctx, "a@example.com", credential(1)); err == nil {
		t.Error("Save on a corrupt file succeeded")
	}
	if err := store.Delete(ctx, "a@example.com"); err == nil {
		t.Error("Delete on a corrupt file succeeded")
	}
	if got, _ := os.ReadFile(path); string(got) != garbage {
		t.Fatalf("corrupt file was modified: %q", got)
	}
}

func TestIncompleteStoredEntryIsRejected(t *testing.T) {
	store, dir := newStore(t)
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"a@example.com":{"version":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background()); err == nil {
		t.Fatal("entry without a hash was accepted")
	}
}

func TestCanceledContextIsHonoured(t *testing.T) {
	store, _ := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Get(ctx, "a@example.com"); !errors.Is(err, context.Canceled) {
		t.Errorf("Get = %v", err)
	}
	if err := store.Save(ctx, "a@example.com", credential(1)); !errors.Is(err, context.Canceled) {
		t.Errorf("Save = %v", err)
	}
	if err := store.Delete(ctx, "a@example.com"); !errors.Is(err, context.Canceled) {
		t.Errorf("Delete = %v", err)
	}
	if _, err := store.List(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("List = %v", err)
	}
}

func TestConcurrentWritersLoseNothingAndReadersNeverSeeATornFile(t *testing.T) {
	store, dir := newStore(t)
	ctx := context.Background()
	const writers = 16
	var wg sync.WaitGroup
	errs := make(chan error, writers*2)
	for i := range writers {
		wg.Add(2)
		go func() {
			defer wg.Done()
			email := fmt.Sprintf("user%d@example.com", i)
			for version := int64(1); version <= 3; version++ {
				if err := store.Save(ctx, email, credential(version)); err != nil {
					errs <- err
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			// A second Store on the same file is how a torn write would show:
			// it shares no mutex, so it only ever sees what rename published.
			other, err := New(dir)
			if err != nil {
				errs <- err
				return
			}
			for range 20 {
				if _, err := other.List(ctx); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent operation failed: %v", err)
	}
	list, err := store.List(ctx)
	if err != nil || len(list) != writers {
		t.Fatalf("List = %d entries, %v; want %d", len(list), err, writers)
	}
	for email, c := range list {
		if c.Version != 3 {
			t.Errorf("%s version = %d, want 3", email, c.Version)
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leaked temp file %s", e.Name())
		}
	}
}
