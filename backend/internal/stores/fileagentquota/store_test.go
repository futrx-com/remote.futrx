package fileagentquota

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentquota "github.com/futrx-com/remote.futrx.com/internal/service/agent/quota"
)

func TestLoadFallbacks(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
	}{
		{name: "missing"}, {name: "unreadable"}, {name: "empty"},
		{name: "malformed", raw: "{"}, {name: "null", raw: "null"},
		// Readings from before plans were kept per account were keyed by
		// provider. They name no account, so they are not carried over.
		{name: "per-provider readings", raw: `{"codex":{"provider":"codex","session":{"window":"session","measuredAt":1}}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			switch test.name {
			case "missing":
			case "unreadable":
				err = os.Mkdir(store.path(), 0o700)
			default:
				err = os.WriteFile(store.path(), []byte(test.raw), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			readings, err := store.Load(context.Background())
			if err != nil || len(readings) != 0 {
				t.Fatalf("Load = %#v, %v", readings, err)
			}
		})
	}
}

func TestSaveRoundTripAndPermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	want := []agentquota.AccountQuota{
		{Provider: "codex", Session: &agent.Quota{
			Window: agent.QuotaWindowSession, UsedPercent: &zero, MeasuredAt: 123,
		}},
		{Provider: "codex", AccountID: "work", Weekly: &agent.Quota{
			Window: agent.QuotaWindowWeekly, Status: "allowed", MeasuredAt: 456,
		}},
	}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background())
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, %v; want %#v", got, err, want)
	}
	for path, mode := range map[string]os.FileMode{root: 0o700, store.path(): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Fatalf("%s mode = %o; want %o", path, info.Mode().Perm(), mode)
		}
	}

	if err := store.Save(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(store.path()); err != nil || string(raw) != "{\n  \"accounts\": []\n}\n" {
		t.Fatalf("empty save = %q, %v", raw, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Save(ctx, nil); err != context.Canceled {
		t.Fatalf("cancelled Save = %v", err)
	}
	if _, err := store.Load(ctx); err != context.Canceled {
		t.Fatalf("cancelled Load = %v", err)
	}
}
