package archives

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
)

func TestPublishWritesTheArchiveIntoAnIgnoredFolder(t *testing.T) {
	root := t.TempDir()
	published, err := New(root, 16, time.Hour).Publish(context.Background(), "src.zip", writePayload("zip"))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if path.Dir(path.Dir(published)) != config.DownloadsDirectory || path.Base(published) != "src.zip" {
		t.Fatalf("published at %q", published)
	}
	if content, err := os.ReadFile(filepath.Join(root, published)); err != nil || string(content) != "zip" {
		t.Fatalf("content = %q, %v", content, err)
	}
	if ignore, err := os.ReadFile(filepath.Join(root, config.DownloadsDirectory, ".gitignore")); err != nil || string(ignore) != "*\n" {
		t.Fatalf(".gitignore = %q, %v", ignore, err)
	}
	if _, err := os.Stat(filepath.Join(root, path.Dir(published), ".partial")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial file remains: %v", err)
	}
}

func TestPublishCapsEachArchiveAndLeavesNothingOnFailure(t *testing.T) {
	root := t.TempDir()
	downloads := New(root, 4, time.Hour)
	if _, err := downloads.Publish(context.Background(), "src.zip", writePayload("12345")); !errors.Is(err, model.ErrArchiveTooLarge) {
		t.Fatalf("oversized error = %v, want %v", err, model.ErrArchiveTooLarge)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := downloads.Publish(ctx, "src.zip", writePayload("1")); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v, want %v", err, context.Canceled)
	}
	if folders := archiveFolders(t, root); len(folders) != 0 {
		t.Fatalf("failed archives left %v", folders)
	}
}

func TestPublishPrunesExpiredArchives(t *testing.T) {
	root := t.TempDir()
	downloads := New(root, 16, time.Hour)
	stale, err := downloads.Publish(context.Background(), "old.zip", writePayload("old"))
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := downloads.Publish(context.Background(), "new.zip", writePayload("new"))
	if err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, path.Dir(stale)), expired, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := downloads.Publish(context.Background(), "next.zip", writePayload("next")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, stale)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired archive remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, fresh)); err != nil {
		t.Fatalf("fresh archive was pruned: %v", err)
	}
}

func TestPublishRefusesASymlinkedDownloadsDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, config.DownloadsDirectory)); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root, 16, time.Hour).Publish(context.Background(), "src.zip", writePayload("zip")); err == nil {
		t.Fatal("published through a symlink")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote outside the workspace: %v", entries)
	}
}

func archiveFolders(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, config.DownloadsDirectory))
	if err != nil {
		t.Fatal(err)
	}
	var folders []string
	for _, entry := range entries {
		if entry.IsDir() {
			folders = append(folders, entry.Name())
		}
	}
	return folders
}

func writePayload(payload string) func(io.Writer) error {
	return func(destination io.Writer) error {
		_, err := io.WriteString(destination, payload)
		return err
	}
}
