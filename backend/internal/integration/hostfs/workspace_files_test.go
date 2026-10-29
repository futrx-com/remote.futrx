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
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0o644); err != nil {
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

	for _, rel := range []string{"../outside-secret.txt", "src/../../outside-secret.txt", "/etc/hosts"} {
		if _, _, _, err := store.OpenFile(root, rel); err == nil {
			t.Fatalf("OpenFile(%q) succeeded, want error", rel)
		}
	}
}

func TestSymlinkInsideWorkspaceIsAllowed(t *testing.T) {
	root, _ := setupWorkspace(t)
	store := NewWorkspaceFileStore()

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

func TestOpenFileRejectsNamedPipeWithoutBlocking(t *testing.T) {
	root, _ := setupWorkspace(t)
	pipePath := filepath.Join(root, "events.pipe")
	if err := syscall.Mkfifo(pipePath, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	store := NewWorkspaceFileStore()

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

func TestOpenFileRejectsEscapingSymlink(t *testing.T) {
	root, secret := setupWorkspace(t)
	if err := os.Symlink(secret, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if file, _, _, err := NewWorkspaceFileStore().OpenFile(root, "escape"); err == nil {
		file.Close()
		t.Fatal("escaping symlink was opened")
	}
}
