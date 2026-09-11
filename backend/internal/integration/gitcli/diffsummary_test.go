package gitcli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func makeTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		command.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
		}
		return strings.TrimSpace(string(output))
	}
	run("init", "-q")
	run("commit", "-q", "--allow-empty", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "add tracked")
	return dir
}

func TestDiffSummaryTalliesTrackedAndUntrackedChanges(t *testing.T) {
	client := NewHistoryClient()
	ctx := context.Background()
	repo := makeTestRepo(t)

	base, err := client.Head(ctx, repo)
	if err != nil {
		t.Fatalf("Head = %v", err)
	}

	if err := os.WriteFile(
		filepath.Join(repo, "tracked.txt"), []byte("one\ntwo\nthree\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "sub", "new.txt"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := client.DiffSummary(ctx, repo, base)
	if err != nil {
		t.Fatalf("DiffSummary = %v", err)
	}
	wantFiles := []string{"sub/new.txt", "tracked.txt"}
	if len(summary.Files) != len(wantFiles) {
		t.Fatalf("Files = %q, want %q", summary.Files, wantFiles)
	}
	for i, want := range wantFiles {
		if summary.Files[i] != want {
			t.Fatalf("Files = %q, want %q", summary.Files, wantFiles)
		}
	}
	if summary.Insertions != 3 || summary.Deletions != 0 {
		t.Fatalf("Insertions/Deletions = %d/%d, want 3/0", summary.Insertions, summary.Deletions)
	}
}

func TestDiffSummaryRejectsUnknownBase(t *testing.T) {
	client := NewHistoryClient()
	repo := makeTestRepo(t)
	if _, err := client.DiffSummary(context.Background(), repo, "deadbeef"); err == nil {
		t.Fatal("DiffSummary = nil, want error for unknown base")
	}
}

func TestDiffSummaryOnMissingDirectory(t *testing.T) {
	client := NewHistoryClient()
	base := filepath.Join(t.TempDir(), "nope")
	if _, err := client.DiffSummary(context.Background(), base, "HEAD"); err == nil {
		t.Fatal("DiffSummary = nil, want error for missing directory")
	}
}
