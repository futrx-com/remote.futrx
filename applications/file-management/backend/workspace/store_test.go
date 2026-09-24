package workspace

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"
)

func setupWorkspace(t *testing.T) (root, secret string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "workspace")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"src/app.go": "package main",
		".env":       "SECRET=1",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	secret = filepath.Join(base, "outside-secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, secret
}

func TestListDirShowsDotfilesAndKeepsBoundedGlobalOrder(t *testing.T) {
	root := t.TempDir()
	for index := directoryReadBatchSize + 20; index >= 0; index-- {
		name := fmt.Sprintf("file-%03d.txt", index)
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"dir-b", "dir-a"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	nodes, truncated, err := NewStore().ListDir(root, "", 4)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("listing did not report truncation")
	}
	got := make([]string, 0, len(nodes))
	for _, node := range nodes {
		got = append(got, node.Name)
	}
	if want := []string{"dir-a", "dir-b", ".env", "file-000.txt"}; !slices.Equal(got, want) {
		t.Fatalf("names = %q, want %q", got, want)
	}
}

func TestStoreBlocksTraversalAndEscapingSymlinks(t *testing.T) {
	root, secret := setupWorkspace(t)
	store := NewStore()
	if err := os.Symlink(secret, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"../outside-secret.txt", "/etc/hosts", "escape"} {
		if file, _, _, _, err := store.OpenFile(root, relative); err == nil {
			_ = file.Close()
			t.Fatalf("OpenFile(%q) escaped workspace", relative)
		}
	}
	nodes, _, err := store.ListDir(root, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range nodes {
		if node.Name == "escape" {
			t.Fatal("escaping symlink was listed")
		}
	}
}

func TestStoreAllowsInWorkspaceSymlinkAndRejectsParentSwap(t *testing.T) {
	root, _ := setupWorkspace(t)
	if err := os.Symlink(filepath.Join(root, "src", "app.go"), filepath.Join(root, "alias.go")); err != nil {
		t.Fatal(err)
	}
	file, _, size, _, err := NewStore().OpenFile(root, "alias.go")
	if err != nil {
		t.Fatal(err)
	}
	content, readErr := io.ReadAll(file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		t.Fatal(err)
	}
	if string(content) != "package main" {
		t.Fatalf("content = %q", content)
	}
	if size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", size, len(content))
	}

	checked := filepath.Join(root, "checked")
	outside := filepath.Join(filepath.Dir(root), "outside")
	if err := os.MkdirAll(checked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checked, "value"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "value"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace, err := newSecureWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.close()
	resolved, err := workspace.resolve("checked/value")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(checked, filepath.Join(root, "checked-original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, checked); err != nil {
		t.Fatal(err)
	}
	opened, err := workspace.root.Open(resolved)
	if opened != nil {
		_ = opened.Close()
		t.Fatal("rooted open followed swapped parent")
	}
	if err == nil {
		t.Fatal("rooted open unexpectedly succeeded")
	}
}

func TestSearchFindsNestedNamesAndDropsEscapes(t *testing.T) {
	root, secret := setupWorkspace(t)
	if err := os.Symlink(secret, filepath.Join(root, "outside-app.txt")); err != nil {
		t.Fatal(err)
	}
	results, _, err := NewStore().Search(root, "app", 100)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, result := range results {
		paths[result.Path] = true
	}
	if !paths["src/app.go"] || paths["outside-app.txt"] {
		t.Fatalf("search paths = %v", paths)
	}
}

func TestArchiveIncludesFilesButDropsEscapesAndSpecialFiles(t *testing.T) {
	root, secret := setupWorkspace(t)
	if err := os.Symlink(secret, filepath.Join(root, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	pipePath := filepath.Join(root, "events.pipe")
	if err := syscall.Mkfifo(pipePath, 0o600); err != nil {
		t.Fatal(err)
	}
	var destination bytes.Buffer
	err := awaitOperation(t, pipePath, func() error {
		return NewStore().WriteArchive(context.Background(), root, "", &destination)
	})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(destination.Bytes()), int64(destination.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, file := range archive.File {
		names[file.Name] = true
	}
	if !names[".env"] || !names["src/app.go"] || names["escape.txt"] || names["events.pipe"] {
		t.Fatalf("archive names = %v", names)
	}
}

func TestArchiveHonorsContextAndSourceBudgets(t *testing.T) {
	root, _ := setupWorkspace(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var destination bytes.Buffer
	if err := NewStore().WriteArchive(ctx, root, "", &destination); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled archive error = %v", err)
	}

	store := NewStore()
	destination.Reset()
	err := store.writeArchive(
		context.Background(), root, "", &destination,
		archiveLimits{maxSourceBytes: 5, maxEntries: 100},
	)
	if !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("source limit error = %v", err)
	}

	destination.Reset()
	err = store.writeArchive(
		context.Background(), root, "", &destination,
		archiveLimits{maxSourceBytes: 1000, maxEntries: 1},
	)
	if !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("entry limit error = %v", err)
	}
}

func TestOpenFileRejectsNamedPipeWithoutBlocking(t *testing.T) {
	root, _ := setupWorkspace(t)
	pipePath := filepath.Join(root, "events.pipe")
	if err := syscall.Mkfifo(pipePath, 0o600); err != nil {
		t.Fatal(err)
	}
	err := awaitOperation(t, pipePath, func() error {
		file, _, _, _, err := NewStore().OpenFile(root, "events.pipe")
		if file != nil {
			_ = file.Close()
		}
		return err
	})
	if err == nil {
		t.Fatal("named pipe was accepted")
	}
}

func awaitOperation(t *testing.T, pipePath string, operation func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- operation() }()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		writerDone := make(chan struct{})
		go func() {
			writer, _ := os.OpenFile(pipePath, os.O_WRONLY, 0)
			if writer != nil {
				_ = writer.Close()
			}
			close(writerDone)
		}()
		<-done
		<-writerDone
		t.Fatal("filesystem operation blocked on named pipe")
		return nil
	}
}
