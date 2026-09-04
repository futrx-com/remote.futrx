package fileusage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
)

func at(year int, month time.Month, day int) int64 {
	return time.Date(year, month, day, 12, 0, 0, 0, time.UTC).UnixMilli()
}

// newStore returns a store plus the data directory it was opened on and the
// usage directory inside it.
func newStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	dataDir := t.TempDir()
	store, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	return store, dataDir, filepath.Join(dataDir, "usage")
}

func collect(t *testing.T, store *Store, from, to int64) []serviceusage.Record {
	t.Helper()
	var out []serviceusage.Record
	if err := store.Scan(context.Background(), from, to, func(record serviceusage.Record) bool {
		out = append(out, record)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAppendRotatesFilesByMonth(t *testing.T) {
	store, _, dir := newStore(t)
	ctx := context.Background()
	records := []serviceusage.Record{
		{At: at(2026, time.July, 30), ChatID: "c1", Provider: "claude", InputTokens: 1},
		{At: at(2026, time.July, 31), ChatID: "c2", Provider: "claude", InputTokens: 2},
		{At: at(2026, time.August, 1), ChatID: "c3", Provider: "codex", InputTokens: 4},
		{At: at(2026, time.December, 31), ChatID: "c4", Provider: "codex", InputTokens: 8},
	}
	for _, record := range records {
		if err := store.Append(ctx, record); err != nil {
			t.Fatal(err)
		}
	}

	for _, month := range []string{"2026-07", "2026-08", "2026-12"} {
		path := filepath.Join(dir, "usage-"+month+".jsonl")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %v, want 0600", path, info.Mode().Perm())
		}
	}

	all := collect(t, store, 0, 0)
	if len(all) != 4 {
		t.Fatalf("scanned %d records, want 4", len(all))
	}
	// Files are visited oldest month first.
	if all[0].ChatID != "c1" || all[3].ChatID != "c4" {
		t.Fatalf("unexpected scan order: %+v", all)
	}
}

func TestScanHonoursTheTimeWindow(t *testing.T) {
	store, _, _ := newStore(t)
	ctx := context.Background()
	for _, record := range []serviceusage.Record{
		{At: at(2026, time.July, 15), ChatID: "july"},
		{At: at(2026, time.August, 15), ChatID: "august"},
		{At: at(2026, time.September, 15), ChatID: "september"},
	} {
		if err := store.Append(ctx, record); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		from, to int64
		want     []string
	}{
		{name: "whole ledger", want: []string{"july", "august", "september"}},
		{
			name: "single month",
			from: at(2026, time.August, 1), to: at(2026, time.August, 31),
			want: []string{"august"},
		},
		{
			name: "spanning two months",
			from: at(2026, time.August, 1), to: at(2026, time.September, 30),
			want: []string{"august", "september"},
		},
		{
			name: "excludes a record inside a matching month",
			from: at(2026, time.August, 16), to: at(2026, time.August, 31),
			want: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := collect(t, store, test.from, test.to)
			if len(got) != len(test.want) {
				t.Fatalf("scanned %d records, want %d: %+v", len(got), len(test.want), got)
			}
			for i, chat := range test.want {
				if got[i].ChatID != chat {
					t.Fatalf("record %d = %q, want %q", i, got[i].ChatID, chat)
				}
			}
		})
	}
}

func TestScanSkipsCorruptLines(t *testing.T) {
	store, _, dir := newStore(t)
	ctx := context.Background()
	if err := store.Append(ctx, serviceusage.Record{At: at(2026, time.August, 1), ChatID: "good"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "usage-2026-08.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{\"at\": trunc\n"); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if err := store.Append(ctx, serviceusage.Record{At: at(2026, time.August, 2), ChatID: "after"}); err != nil {
		t.Fatal(err)
	}

	got := collect(t, store, 0, 0)
	if len(got) != 2 || got[0].ChatID != "good" || got[1].ChatID != "after" {
		t.Fatalf("unexpected records: %+v", got)
	}
}

func TestReplaceAllRewritesAndPrunesMonths(t *testing.T) {
	store, _, dir := newStore(t)
	ctx := context.Background()
	for _, record := range []serviceusage.Record{
		{At: at(2026, time.July, 1), ChatID: "old"},
		{At: at(2026, time.August, 1), ChatID: "keep"},
	} {
		if err := store.Append(ctx, record); err != nil {
			t.Fatal(err)
		}
	}

	months, err := store.ReplaceAll(ctx, []serviceusage.Record{
		{At: at(2026, time.August, 1), ChatID: "keep"},
		{At: at(2026, time.August, 2), ChatID: "new"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(months) != 1 || months[0] != "2026-08" {
		t.Fatalf("months = %v, want [2026-08]", months)
	}
	if _, err := os.Stat(filepath.Join(dir, "usage-2026-07.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("expected the July file to be pruned, stat err = %v", err)
	}
	got := collect(t, store, 0, 0)
	if len(got) != 2 || got[0].ChatID != "keep" || got[1].ChatID != "new" {
		t.Fatalf("unexpected records: %+v", got)
	}
}

func TestPricesSeedDefaultsOnFirstRead(t *testing.T) {
	store, _, dir := newStore(t)
	table, err := store.Prices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Models) == 0 || table.Currency != "USD" {
		t.Fatalf("unexpected seeded table: %+v", table)
	}
	path := filepath.Join(dir, "prices.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected %s to be written: %v", path, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("prices.json mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestSetPricesReplacesTheTable(t *testing.T) {
	store, dataDir, _ := newStore(t)
	ctx := context.Background()
	saved, err := store.SetPrices(ctx, serviceusage.PriceTable{
		UpdatedAt: 1234,
		Models:    []serviceusage.ModelPrice{{Match: "House-Model", InputPerMTok: 7, OutputPerMTok: 9}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Models) != 1 || saved.Models[0].Match != "house-model" || saved.UpdatedAt != 1234 {
		t.Fatalf("unexpected saved table: %+v", saved)
	}

	reopened, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reopened.Prices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Models) != 1 || reloaded.Models[0].InputPerMTok != 7 {
		t.Fatalf("table did not survive a reopen: %+v", reloaded)
	}

	if _, err := store.SetPrices(ctx, serviceusage.PriceTable{}); err == nil {
		t.Fatal("expected an empty table to be rejected")
	}
}
