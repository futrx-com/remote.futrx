package fileusers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/user"
)

func newStore(t *testing.T, dataDir string) *Store {
	t.Helper()
	store, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestAddNormalizesTheEmailAndGetIgnoresCase(t *testing.T) {
	store := newStore(t, t.TempDir())
	ctx := context.Background()

	if err := store.Add(ctx, user.User{Email: " Alice@Example.com ", Role: user.RoleMember, AddedBy: "admin@example.com"}); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get(ctx, "ALICE@example.COM")
	if err != nil || got == nil {
		t.Fatalf("Get() = %#v, %v; want the user", got, err)
	}
	if got.Email != "alice@example.com" || got.Role != user.RoleMember || got.AddedBy != "admin@example.com" {
		t.Fatalf("Get() = %#v, want the normalized, stored user", got)
	}
	if missing, err := store.Get(ctx, "nobody@example.com"); err != nil || missing != nil {
		t.Fatalf("Get(unknown) = %#v, %v; want nil, nil", missing, err)
	}
}

func TestAddAndRemoveReportTheDocumentedErrors(t *testing.T) {
	store := newStore(t, t.TempDir())
	ctx := context.Background()
	if err := store.Add(ctx, user.User{Email: "alice@example.com"}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "duplicate add", err: store.Add(ctx, user.User{Email: "ALICE@example.com"}), want: user.ErrUserExists},
		{name: "blank add", err: store.Add(ctx, user.User{Email: "  "}), want: user.ErrInvalidEmail},
		{name: "remove unknown", err: store.Remove(ctx, "nobody@example.com"), want: user.ErrUserNotFound},
		{name: "remove blank", err: store.Remove(ctx, ""), want: user.ErrInvalidEmail},
		{name: "role for unknown", err: store.SetRole(ctx, "nobody@example.com", user.RoleAdmin), want: user.ErrUserNotFound},
		{name: "role for blank", err: store.SetRole(ctx, " ", user.RoleAdmin), want: user.ErrInvalidEmail},
	}
	for _, test := range tests {
		if !errors.Is(test.err, test.want) {
			t.Errorf("%s: error = %v, want %v", test.name, test.err, test.want)
		}
	}
}

func TestSetRoleRemoveAndCountPersistAcrossANewStore(t *testing.T) {
	dataDir := t.TempDir()
	ctx := context.Background()
	store := newStore(t, dataDir)
	for _, email := range []string{"carol@example.com", "alice@example.com", "bob@example.com"} {
		if err := store.Add(ctx, user.User{Email: email, Role: user.RoleMember}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SetRole(ctx, "Bob@example.com", user.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(ctx, "carol@example.com"); err != nil {
		t.Fatal(err)
	}

	reopened := newStore(t, dataDir)
	users, err := reopened.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || users[0].Email != "alice@example.com" || users[1].Email != "bob@example.com" {
		t.Fatalf("List() after reopening = %#v, want alice then bob", users)
	}
	if users[1].Role != user.RoleAdmin {
		t.Fatalf("bob role = %q, want admin", users[1].Role)
	}
	if count, err := reopened.Count(ctx); err != nil || count != 2 {
		t.Fatalf("Count() = %d, %v; want 2", count, err)
	}
}

func TestACorruptUsersFileIsReportedNotOverwritten(t *testing.T) {
	dataDir := t.TempDir()
	store := newStore(t, dataDir)
	file := filepath.Join(dataDir, "users.json")
	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Count(context.Background()); err == nil {
		t.Fatal("Count() error = nil, want a parse error")
	}
	if err := store.Add(context.Background(), user.User{Email: "alice@example.com"}); err == nil {
		t.Fatal("Add() error = nil, want the parse error rather than replacing every user")
	}
	if raw, _ := os.ReadFile(file); string(raw) != "{not json" {
		t.Fatalf("users.json = %q, want it left as it was", raw)
	}
}

func TestConcurrentAddsKeepEveryUser(t *testing.T) {
	store := newStore(t, t.TempDir())
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.Add(context.Background(), user.User{Email: fmt.Sprintf("user%02d@example.com", i)}); err != nil {
				t.Errorf("Add() error = %v", err)
			}
		}()
	}
	wg.Wait()

	if count, err := store.Count(context.Background()); err != nil || count != 20 {
		t.Fatalf("Count() = %d, %v; want 20", count, err)
	}
}
