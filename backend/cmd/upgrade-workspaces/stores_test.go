package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/stores/sqlitechat"
)

func TestWorkspaceStoresWorkWhileBackendOwnsConversationStorage(t *testing.T) {
	root := t.TempDir()
	backendChats, err := sqlitechat.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backendChats.Close() })
	if err := backendChats.EnsureRuntimeReady(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if other, err := sqlitechat.New(root); err == nil {
		_ = other.Close()
		t.Fatal("fixture did not hold the backend's conversation lock")
	}

	stores, err := newWorkspaceStores(root, t.TempDir())
	if err != nil {
		t.Fatalf("workspace maintenance conflicted with active backend: %v", err)
	}
	ctx := context.Background()
	if _, err := stores.Projects.List(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := stores.ProjectSecrets.List(ctx, "abcdef"); err != nil {
		t.Fatal(err)
	}
	if _, err := stores.ProjectAccess.List(ctx, "abcdef"); err != nil {
		t.Fatal(err)
	}
	if _, err := stores.Applications.ListProject(ctx, "abcdef"); err != nil {
		t.Fatal(err)
	}
	if err := backendChats.DB().PingContext(ctx); err != nil {
		t.Fatalf("workspace maintenance disturbed backend database: %v", err)
	}
}

func TestWorkspaceStoresDoNotCreateConversationStorage(t *testing.T) {
	root := t.TempDir()
	if _, err := newWorkspaceStores(root, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{sqlitechat.Filename, ".conversations.lock", "transcript-index.sqlite", "chats"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Errorf("workspace maintenance touched %s: %v", name, err)
		}
	}
}
