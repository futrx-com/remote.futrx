package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	usage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileusage"
	"github.com/futrx-com/remote.futrx.com/internal/stores/sqlitechat"
)

func TestRebuildReadsSQLiteAndNeverRecreatesLegacyIndex(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := sqlitechat.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureRuntimeReady(ctx, root); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, chat.Meta{ID: "abcd", Provider: chat.ProviderCodex}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(ctx, "abcd", chat.Event{Type: "complete", T: 1795114492991, TurnID: "new-sqlite-run"}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	// An invalid stale backup ensures an accidental legacy read cannot pass.
	legacy := filepath.Join(root, "chats", "dcba")
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "meta.json"), []byte("invalid inactive backup"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	ledger, err := fileusage.New(root)
	if err != nil {
		t.Fatal(err)
	}
	var records []usage.Record
	if err := ledger.Scan(ctx, 0, 0, func(record usage.Record) bool { records = append(records, record); return true }); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ChatID != "abcd" || records[0].RunID != "new-sqlite-run" {
		t.Fatalf("records=%+v", records)
	}
	if _, err := os.Stat(filepath.Join(root, "transcript-index.sqlite")); !os.IsNotExist(err) {
		t.Fatal("legacy index recreated")
	}
	// Close released the exclusive lease: consecutive offline rebuilds work.
	if err := run(ctx, root, true); err != nil {
		t.Fatal(err)
	}
}

func TestRebuildRequiresStoppedBackendAndCompletedMigration(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := sqlitechat.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, root, true); err == nil || !strings.Contains(err.Error(), "stop the backend") {
		t.Fatalf("live backend error=%v", err)
	}
	store.Close()
	if err := os.MkdirAll(filepath.Join(root, "chats", "abcd"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, root, false); err == nil || !strings.Contains(err.Error(), "migration required") {
		t.Fatalf("legacy migration error=%v", err)
	}
}
