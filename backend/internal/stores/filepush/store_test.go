package filepush

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
)

func subscription(endpoint string, createdAt int64) servicepush.Subscription {
	return servicepush.Subscription{
		Endpoint:  endpoint,
		P256dh:    "p256dh-" + endpoint,
		Auth:      "auth-" + endpoint,
		CreatedAt: createdAt,
	}
}

func TestStoreRoundTripsSubscriptionsUnderAHashedFilename(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if err := store.Save(ctx, "Ops@Example.com", subscription("https://push.example.com/a", 10)); err != nil {
		t.Fatal(err)
	}

	// Lookup is case-insensitive because the key is the normalized email.
	got, err := store.List(ctx, "ops@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Endpoint != "https://push.example.com/a" {
		t.Fatalf("got %+v", got)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "push-subscriptions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one file, got %d", len(entries))
	}
	// The filename must not disclose which accounts exist on this server.
	if name := entries[0].Name(); !strings.HasPrefix(name, "sha256-") || strings.Contains(name, "example.com") {
		t.Fatalf("filename %q leaks the account", name)
	}
}

func TestListIsEmptyForAnUnknownUser(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.List(context.Background(), "nobody@example.com")
	if err != nil {
		t.Fatalf("unknown user should not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestSaveReplacesAKnownEndpointAndKeepsCreationOrder(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	_ = store.Save(ctx, "ops@example.com", subscription("https://push.example.com/b", 20))
	_ = store.Save(ctx, "ops@example.com", subscription("https://push.example.com/a", 10))

	refreshed := subscription("https://push.example.com/b", 20)
	refreshed.LastSentAt = 999
	if err := store.Save(ctx, "ops@example.com", refreshed); err != nil {
		t.Fatal(err)
	}

	got, _ := store.List(ctx, "ops@example.com")
	if len(got) != 2 {
		t.Fatalf("got %d subscriptions, want 2", len(got))
	}
	if got[0].Endpoint != "https://push.example.com/a" {
		t.Fatalf("subscriptions are not ordered by creation: %+v", got)
	}
	if got[1].LastSentAt != 999 {
		t.Fatal("the refreshed subscription did not replace the stored one")
	}
}

func TestDeleteRemovesTheFileOnceTheLastDeviceIsGone(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	_ = store.Save(ctx, "ops@example.com", subscription("https://push.example.com/a", 10))
	_ = store.Save(ctx, "ops@example.com", subscription("https://push.example.com/b", 20))

	if err := store.Delete(ctx, "ops@example.com", "https://push.example.com/a"); err != nil {
		t.Fatal(err)
	}
	got, _ := store.List(ctx, "ops@example.com")
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}

	if err := store.Delete(ctx, "ops@example.com", "https://push.example.com/b"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "push-subscriptions"))
	if len(entries) != 0 {
		t.Fatalf("expected the empty record to be removed, found %d files", len(entries))
	}
}

func TestDeletingAnUnknownEndpointIsNotAnError(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "ops@example.com", "https://push.example.com/nope"); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteAllRemovesEveryDeviceAndIsIdempotent(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = store.Save(ctx, "ops@example.com", subscription("https://push.example.com/a", 10))
	_ = store.Save(ctx, "ops@example.com", subscription("https://push.example.com/b", 20))

	for i := 0; i < 2; i++ {
		if err := store.DeleteAll(ctx, "OPS@example.com"); err != nil {
			t.Fatalf("delete all %d: %v", i, err)
		}
	}
	if subscriptions, err := store.List(ctx, "ops@example.com"); err != nil || len(subscriptions) != 0 {
		t.Fatalf("subscriptions = %+v, %v; want empty", subscriptions, err)
	}
}

func TestARecordRequiresAnIdentity(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background(), "   "); !errors.Is(err, servicepush.ErrInvalidIdentity) {
		t.Fatalf("err = %v, want ErrInvalidIdentity", err)
	}
}

func TestVAPIDKeysAreMintedOnceAndReusedThereafter(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	generate := func() (string, string, error) {
		calls++
		return "private-key", "public-key", nil
	}

	private, public, err := store.VAPIDKeys(generate)
	if err != nil || private != "private-key" || public != "public-key" {
		t.Fatalf("first call: %q %q %v", private, public, err)
	}

	// A second process start must reuse the stored pair: regenerating it
	// would silently invalidate every browser subscription.
	reopened, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	private, public, err = reopened.VAPIDKeys(generate)
	if err != nil || private != "private-key" || public != "public-key" {
		t.Fatalf("second call: %q %q %v", private, public, err)
	}
	if calls != 1 {
		t.Fatalf("generate called %d times, want 1", calls)
	}

	info, err := os.Stat(filepath.Join(dir, vapidFile))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("vapid key file mode = %v, want 0600", mode)
	}
}

func TestVAPIDKeysSurfaceGenerationFailures(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.VAPIDKeys(func() (string, string, error) {
		return "", "", errors.New("no entropy")
	})
	if err == nil {
		t.Fatal("expected the generation failure to surface")
	}
}
