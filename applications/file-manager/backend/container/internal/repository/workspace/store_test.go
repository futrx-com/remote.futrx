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

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	model "futrx.local/catalog/applications/file-manager/backend/container/internal/model/files/application"
)

// setupWorkspace builds a workspace tree plus an out-of-workspace secret and
// returns the workspace root.
func setupWorkspace(t *testing.T) (root, secret string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "workspace")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0o644); err != nil {
		t.Fatal(err)
	}
	secret = filepath.Join(base, "outside-secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, secret
}

func TestListDirShowsDotfiles(t *testing.T) {
	root, _ := setupWorkspace(t)
	store := New()

	nodes, _, err := store.ListDir(root, "", 100)
	if err != nil {
		t.Fatalf("ListDir root: %v", err)
	}
	names := map[string]bool{}
	for _, n := range nodes {
		names[n.Name] = true
	}
	if !names[".env"] {
		t.Fatalf("expected dotfile .env to be listed, got %v", names)
	}
	if !names["src"] {
		t.Fatalf("expected src dir to be listed, got %v", names)
	}
}

func TestListDirPreservesGlobalOrderAcrossReadBatches(t *testing.T) {
	root := t.TempDir()
	for index := config.DirectoryReadBatchSize + 20; index >= 0; index-- {
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
	store := New()

	nodes, truncated, err := store.ListDir(root, "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("ListDir() did not report truncation")
	}
	want := []string{"dir-a", "dir-b", "file-000.txt"}
	got := make([]string, 0, len(nodes))
	for _, node := range nodes {
		got = append(got, node.Name)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ListDir() names = %q, want %q", got, want)
	}
}

func TestOpenFileRejectsTraversal(t *testing.T) {
	root, _ := setupWorkspace(t)
	store := New()

	for _, rel := range []string{"../outside-secret.txt", "src/../../outside-secret.txt", "/etc/hosts"} {
		if _, _, _, err := store.OpenFile(root, rel); err == nil {
			t.Fatalf("OpenFile(%q) succeeded, want error", rel)
		}
	}
}

func TestSymlinkEscapeIsBlocked(t *testing.T) {
	root, secret := setupWorkspace(t)
	store := New()

	// Simulate untrusted container code planting a symlink to a host file.
	link := filepath.Join(root, "escape")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// Downloading the escaping symlink must fail.
	if _, _, _, err := store.OpenFile(root, "escape"); err == nil {
		t.Fatal("OpenFile via escaping symlink succeeded, want error")
	}

	// And it must not appear as a usable entry in the listing.
	nodes, _, err := store.ListDir(root, "", 100)
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	for _, n := range nodes {
		if n.Name == "escape" {
			t.Fatal("escaping symlink was listed, want it dropped")
		}
	}
}

func TestSymlinkInsideWorkspaceIsAllowed(t *testing.T) {
	root, _ := setupWorkspace(t)
	store := New()

	link := filepath.Join(root, "alias.go")
	if err := os.Symlink(filepath.Join(root, "src", "app.go"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	rc, _, _, err := store.OpenFile(root, "alias.go")
	if err != nil {
		t.Fatalf("OpenFile via in-workspace symlink: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "package main" {
		t.Fatalf("unexpected content %q", data)
	}
}

func TestRootedOpenRejectsParentSymlinkSwapAfterResolve(t *testing.T) {
	root, _ := setupWorkspace(t)
	checkedParent := filepath.Join(root, "checked")
	if err := os.MkdirAll(checkedParent, 0o755); err != nil {
		t.Fatalf("mkdir checked parent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(checkedParent, "secret.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatalf("write checked file: %v", err)
	}
	outside := filepath.Join(filepath.Dir(root), "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	workspace, err := newSecureWorkspace(root)
	if err != nil {
		t.Fatalf("open secure workspace: %v", err)
	}
	defer workspace.close()
	resolved, err := workspace.resolve("checked/secret.txt")
	if err != nil {
		t.Fatalf("resolve checked file: %v", err)
	}
	if err := os.Rename(checkedParent, filepath.Join(root, "checked-original")); err != nil {
		t.Fatalf("rename checked parent: %v", err)
	}
	if err := os.Symlink(outside, checkedParent); err != nil {
		t.Fatalf("replace checked parent with symlink: %v", err)
	}

	file, err := workspace.root.Open(resolved)
	if file != nil {
		_ = file.Close()
		t.Fatal("rooted open followed swapped parent outside workspace")
	}
	if err == nil {
		t.Fatal("rooted open unexpectedly succeeded after parent symlink swap")
	}
}

func TestSearchFindsNestedFile(t *testing.T) {
	root, _ := setupWorkspace(t)
	store := New()

	results, _, err := store.Search(root, "app", 100)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var found bool
	for _, n := range results {
		if n.Path == "src/app.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected to find src/app.go, got %v", results)
	}
}

func TestSearchDropsSymlinkThatEscapesWorkspace(t *testing.T) {
	root, secret := setupWorkspace(t)
	store := New()
	if err := os.Symlink(secret, filepath.Join(root, "outside-match.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	results, _, err := store.Search(root, "outside", 100)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, result := range results {
		if result.Name == "outside-match.txt" {
			t.Fatal("escaping symlink appeared in search results")
		}
	}
}

func TestWriteArchiveIncludesWorkspaceFilesAndDropsEscapingSymlinks(t *testing.T) {
	root, secret := setupWorkspace(t)
	store := New()
	if err := os.Symlink(secret, filepath.Join(root, "escape.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var destination bytes.Buffer
	if err := store.WriteArchive(context.Background(), root, "", &destination); err != nil {
		t.Fatalf("WriteArchive: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(destination.Bytes()), int64(destination.Len()))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	names := make(map[string]bool, len(archive.File))
	for _, file := range archive.File {
		names[file.Name] = true
	}
	if !names[".env"] || !names["src/app.go"] {
		t.Fatalf("archive is missing workspace files: %v", names)
	}
	if names["escape.txt"] {
		t.Fatal("archive included symlink that escapes the workspace")
	}
}

func TestWriteArchiveIncludesAbsoluteSymlinkWithinWorkspace(t *testing.T) {
	root, _ := setupWorkspace(t)
	store := New()
	if err := os.Symlink(filepath.Join(root, "src", "app.go"), filepath.Join(root, "alias.go")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var destination bytes.Buffer
	if err := store.WriteArchive(context.Background(), root, "", &destination); err != nil {
		t.Fatalf("WriteArchive: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(destination.Bytes()), int64(destination.Len()))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	for _, file := range archive.File {
		if file.Name != "alias.go" {
			continue
		}
		content, err := file.Open()
		if err != nil {
			t.Fatalf("open alias.go: %v", err)
		}
		data, readErr := io.ReadAll(content)
		closeErr := content.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			t.Fatalf("read alias.go: %v", err)
		}
		if string(data) != "package main" {
			t.Fatalf("alias.go content = %q", data)
		}
		return
	}
	t.Fatal("archive omitted in-workspace absolute symlink")
}

func TestOpenFileRejectsNamedPipeWithoutBlocking(t *testing.T) {
	root, _ := setupWorkspace(t)
	pipePath := filepath.Join(root, "events.pipe")
	if err := syscall.Mkfifo(pipePath, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	store := New()

	err := awaitPipeOperation(t, pipePath, func() error {
		file, _, _, err := store.OpenFile(root, "events.pipe")
		if file != nil {
			_ = file.Close()
		}
		return err
	})
	if err == nil {
		t.Fatal("OpenFile accepted a named pipe")
	}
}

func TestWriteArchiveSkipsNamedPipeWithoutBlocking(t *testing.T) {
	root, _ := setupWorkspace(t)
	pipePath := filepath.Join(root, "events.pipe")
	if err := syscall.Mkfifo(pipePath, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	store := New()
	var destination bytes.Buffer

	err := awaitPipeOperation(t, pipePath, func() error {
		return store.WriteArchive(context.Background(), root, "", &destination)
	})
	if err != nil {
		t.Fatalf("WriteArchive: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(destination.Bytes()), int64(destination.Len()))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	for _, file := range archive.File {
		if file.Name == "events.pipe" {
			t.Fatal("archive included a named pipe")
		}
	}
}

func TestWriteArchiveHonorsCanceledContext(t *testing.T) {
	root, _ := setupWorkspace(t)
	store := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var destination bytes.Buffer

	err := store.WriteArchive(ctx, root, "", &destination)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WriteArchive error = %v, want %v", err, context.Canceled)
	}
	if destination.Len() != 0 {
		t.Fatalf("canceled archive wrote %d bytes", destination.Len())
	}
}

func TestWriteArchiveStopsWhenContextIsCanceledDuringBuild(t *testing.T) {
	root, _ := setupWorkspace(t)
	store := New()
	ctx, cancel := context.WithCancel(context.Background())
	destination := &cancelAfterFirstWrite{cancel: cancel}

	err := store.WriteArchive(ctx, root, "", destination)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WriteArchive error = %v, want %v", err, context.Canceled)
	}
}

func TestWriteArchiveReturnsDestinationFailure(t *testing.T) {
	root, _ := setupWorkspace(t)
	store := New()
	wantErr := errors.New("destination failed")

	err := store.WriteArchive(context.Background(), root, "", failingWriter{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WriteArchive error = %v, want %v", err, wantErr)
	}
}

func TestWriteArchiveRejectsCumulativeUncompressedBytes(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{"a.txt": "123", "b.txt": "456"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := New()
	var destination bytes.Buffer

	err := store.writeArchive(
		context.Background(),
		root,
		"",
		&destination,
		archiveLimits{maxSourceBytes: 5, maxEntries: 10},
	)
	if !errors.Is(err, model.ErrArchiveTooLarge) {
		t.Fatalf("WriteArchive error = %v, want %v", err, model.ErrArchiveTooLarge)
	}
}

func TestWriteArchiveRejectsTooManyEntries(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := New()
	var destination bytes.Buffer

	err := store.writeArchive(
		context.Background(),
		root,
		"",
		&destination,
		archiveLimits{maxSourceBytes: 100, maxEntries: 1},
	)
	if !errors.Is(err, model.ErrArchiveTooLarge) {
		t.Fatalf("WriteArchive error = %v, want %v", err, model.ErrArchiveTooLarge)
	}
}

func TestWriteArchiveEntryLimitIncludesDirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store := New()
	var destination bytes.Buffer

	err := store.writeArchive(
		context.Background(),
		root,
		"",
		&destination,
		archiveLimits{maxSourceBytes: 100, maxEntries: 1},
	)
	if !errors.Is(err, model.ErrArchiveTooLarge) {
		t.Fatalf("WriteArchive error = %v, want %v", err, model.ErrArchiveTooLarge)
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type cancelAfterFirstWrite struct {
	cancel context.CancelFunc
	wrote  bool
}

func (w *cancelAfterFirstWrite) Write(buffer []byte) (int, error) {
	if !w.wrote {
		w.wrote = true
		w.cancel()
	}
	return len(buffer), nil
}

func awaitPipeOperation(t *testing.T, pipePath string, operation func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- operation()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		// Pair with an accidentally blocking reader so the test can clean up before
		// reporting the regression instead of leaking a stuck goroutine.
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
		t.Fatal("filesystem operation blocked while opening a named pipe")
		return nil
	}
}
