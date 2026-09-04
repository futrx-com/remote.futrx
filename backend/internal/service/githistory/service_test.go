package githistory

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/integration/gitcli"
)

func TestRepositoryDiscoverySkipsProviderCompatibilityHomes(t *testing.T) {
	for _, directory := range []string{".claude", ".codex", ".minimax"} {
		if !slices.Contains(skippedDirectories, directory) {
			t.Fatalf("skippedDirectories is missing %q", directory)
		}
	}
}

func TestResolveRepositoryPath(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	makeGitRepo(t, workspaceRoot)
	makeGitRepo(t, filepath.Join(workspaceRoot, "packages", "app"))
	nestedRepo := filepath.Join(workspaceRoot, "packages", "app")
	service := New(gitcli.NewHistoryClient())
	tests := []struct {
		name          string
		rawRepository string
		want          string
	}{
		{name: "root", rawRepository: ".", want: workspaceRoot},
		{name: "relative", rawRepository: "packages/app", want: nestedRepo},
		{name: "container", rawRepository: "/workspace/packages/app", want: nestedRepo},
		{name: "host", rawRepository: nestedRepo, want: nestedRepo},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := service.resolveRepositoryPath(test.rawRepository, workspaceRoot)
			if err != nil {
				t.Fatalf("resolveRepositoryPath() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("resolveRepositoryPath() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveRepositoryPathRejectsUnsafePaths(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	makeGitRepo(t, workspaceRoot)
	service := New(gitcli.NewHistoryClient())
	tests := []string{"../other", "/workspace/../etc", filepath.Join(filepath.Dir(workspaceRoot), "other"), "/etc"}

	for _, rawRepository := range tests {
		t.Run(rawRepository, func(t *testing.T) {
			if got, err := service.resolveRepositoryPath(rawRepository, workspaceRoot); err == nil {
				t.Fatalf("resolveRepositoryPath() = %q, want error", got)
			}
		})
	}
}

func TestParseCommitLine(t *testing.T) {
	line := "0123456789abcdef\x1f0123456\x1fAda Lovelace\x1fada@example.com\x1f1712345678\x1fAdd history drawer"
	commit, err := parseCommitLine(line, "0123456789abcdef")
	if err != nil {
		t.Fatalf("parseCommitLine() error = %v", err)
	}
	if !commit.IsHead || commit.ShortSHA != "0123456" || commit.Subject != "Add history drawer" || commit.AuthorDate != 1712345678 {
		t.Fatalf("parseCommitLine() = %+v", commit)
	}
}

func TestParseDirtyFiles(t *testing.T) {
	status := " M app.tsx\n?? new-file.txt\nR  old.txt -> new.txt\n"
	files := parseDirtyFiles(status)
	want := []string{"M app.tsx", "?? new-file.txt", "R  old.txt -> new.txt"}
	if strings.Join(files, "|") != strings.Join(want, "|") {
		t.Fatalf("parseDirtyFiles() = %#v, want %#v", files, want)
	}
}

func TestSanitizeCheckpointMessage(t *testing.T) {
	got := sanitizeCheckpointMessage("  checkpoint\n before\t switch  ")
	if got != "checkpoint before switch" {
		t.Fatalf("sanitizeCheckpointMessage() = %q", got)
	}
}

func TestCheckpointChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repository := t.TempDir()
	runTestGit(t, repository, "init")
	if err := os.WriteFile(filepath.Join(repository, "app.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repository, "add", "app.txt")
	runTestGit(t, repository, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(repository, "app.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	service := New(gitcli.NewHistoryClient())
	sha, err := service.checkpointChanges(context.Background(), repository, "  checkpoint\n before switch  ")
	if err != nil {
		t.Fatalf("checkpointChanges() error = %v", err)
	}
	if sha == "" {
		t.Fatal("checkpointChanges() returned empty sha")
	}
	status, dirtyFiles, err := service.repositoryStatus(context.Background(), repository)
	if err != nil {
		t.Fatalf("repositoryStatus() error = %v", err)
	}
	if status != "" || len(dirtyFiles) != 0 {
		t.Fatalf("status = %q, dirtyFiles = %#v; want clean", status, dirtyFiles)
	}
	message := runTestGit(t, repository, "log", "-1", "--pretty=%s")
	if strings.TrimSpace(message) != "checkpoint before switch" {
		t.Fatalf("commit subject = %q", message)
	}
}

func makeGitRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatalf("make git repo: %v", err)
	}
}

func runTestGit(t *testing.T, repository string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repository}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
}
