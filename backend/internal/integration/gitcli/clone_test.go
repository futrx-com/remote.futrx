package gitcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func newFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	run("-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-q", "-m", "init")
	return dir
}

func TestCloneIntoEmptyDestination(t *testing.T) {
	source := newFixtureRepo(t)
	dest := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := NewCloner().Clone(context.Background(), source, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		t.Fatalf("cloned repo missing .git: %v", err)
	}
}

func TestCloneSkipsAnAlreadySeededDestination(t *testing.T) {
	source := newFixtureRepo(t)
	dest := t.TempDir()
	marker := filepath.Join(dest, "keep-me.txt")
	if err := os.WriteFile(marker, []byte("existing work"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := NewCloner().Clone(context.Background(), source, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		t.Fatal("cloned into a non-empty destination")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "existing work" {
		t.Fatalf("existing content was disturbed: %q, %v", data, err)
	}
}

func TestCloneReturnsCleanErrorOnFailure(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	err := NewCloner().Clone(context.Background(), filepath.Join(t.TempDir(), "does-not-exist"), dest)
	if !errors.Is(err, ErrCloneFailed) {
		t.Fatalf("error = %v, want %v", err, ErrCloneFailed)
	}
}
