package hostfs

import (
	"io"
	"os"
	"path/filepath"
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
	if err := os.WriteFile(filepath.Join(root, "src", "app.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	secret = filepath.Join(base, "outside-secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, secret
}

func TestOpenFileRejectsTraversal(t *testing.T) {
	root, _ := setupWorkspace(t)
	store := NewWorkspaceFileStore()
	for _, relative := range []string{"../outside-secret.txt", "src/../../outside-secret.txt", "/etc/hosts"} {
		if _, _, _, err := store.OpenFile(root, relative); err == nil {
			t.Fatalf("OpenFile(%q) succeeded, want error", relative)
		}
	}
}

func TestOpenFileBlocksEscapingSymlink(t *testing.T) {
	root, secret := setupWorkspace(t)
	if err := os.Symlink(secret, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := NewWorkspaceFileStore().OpenFile(root, "escape"); err == nil {
		t.Fatal("OpenFile via escaping symlink succeeded")
	}
}

func TestOpenFileAllowsSymlinkInsideWorkspace(t *testing.T) {
	root, _ := setupWorkspace(t)
	if err := os.Symlink(filepath.Join(root, "src", "app.go"), filepath.Join(root, "alias.go")); err != nil {
		t.Fatal(err)
	}
	file, _, _, err := NewWorkspaceFileStore().OpenFile(root, "alias.go")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil || string(content) != "package main" {
		t.Fatalf("content = %q, error = %v", content, err)
	}
}

func TestRootedOpenRejectsParentSymlinkSwapAfterResolve(t *testing.T) {
	root, _ := setupWorkspace(t)
	checked := filepath.Join(root, "checked")
	if err := os.MkdirAll(checked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checked, "secret.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(root), "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}

	workspace, err := newSecureWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.close()
	resolved, err := workspace.resolve("checked/secret.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(checked, filepath.Join(root, "checked-original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, checked); err != nil {
		t.Fatal(err)
	}
	file, err := workspace.root.Open(resolved)
	if file != nil {
		_ = file.Close()
		t.Fatal("rooted open followed the swapped parent")
	}
	if err == nil {
		t.Fatal("rooted open unexpectedly succeeded")
	}
}

func TestOpenFileRejectsNamedPipeWithoutBlocking(t *testing.T) {
	root, _ := setupWorkspace(t)
	pipe := filepath.Join(root, "events.pipe")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		file, _, _, err := NewWorkspaceFileStore().OpenFile(root, "events.pipe")
		if file != nil {
			_ = file.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("OpenFile accepted a named pipe")
		}
	case <-time.After(time.Second):
		t.Fatal("OpenFile blocked on a named pipe")
	}
}
