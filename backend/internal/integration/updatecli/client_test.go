package updatecli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceselfupdate "github.com/futrx-com/remote.futrx.com/internal/service/selfupdate"
)

func TestListRemoteTagsForCommitResolvesLightweightAndAnnotatedTags(t *testing.T) {
	originDir := filepath.Join(t.TempDir(), "origin.git")
	seedDir := filepath.Join(t.TempDir(), "seed")
	runGit(t, "", "init", "--bare", "--quiet", originDir)
	runGit(t, "", "init", "--quiet", seedDir)
	runGit(t, seedDir, "config", "user.name", "Update Test")
	runGit(t, seedDir, "config", "user.email", "update-test@example.invalid")
	if err := os.WriteFile(filepath.Join(seedDir, "version"), []byte("0.20.1"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, seedDir, "add", "version")
	runGit(t, seedDir, "commit", "--quiet", "-m", "0.20.1")
	oldCommit := runGit(t, seedDir, "rev-parse", "HEAD")
	runGit(t, seedDir, "tag", "0.20.1")
	runGit(t, seedDir, "tag", "-a", "0.20.1.1", "-m", "release alias")
	if err := os.WriteFile(filepath.Join(seedDir, "version"), []byte("0.20.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, seedDir, "commit", "--quiet", "-am", "0.20.4")
	runGit(t, seedDir, "tag", "0.20.4")
	runGit(t, seedDir, "remote", "add", "origin", originDir)
	runGit(t, seedDir, "push", "--quiet", "origin", "HEAD", "--tags")

	tags, err := (Client{}).ListRemoteTagsForCommit(context.Background(), seedDir, oldCommit[:7])
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(tags, ","), "0.20.1,0.20.1.1"; got != want {
		t.Fatalf("matching tags = %q, want %q", got, want)
	}
}

func TestReadReleaseNotesFetchesExactTagWithoutChangingCheckout(t *testing.T) {
	originDir := filepath.Join(t.TempDir(), "origin.git")
	seedDir := filepath.Join(t.TempDir(), "seed")
	installDir := filepath.Join(t.TempDir(), "install")
	runGit(t, "", "init", "--bare", "--quiet", originDir)
	runGit(t, originDir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGit(t, "", "init", "--quiet", seedDir)
	runGit(t, seedDir, "config", "user.name", "Update Test")
	runGit(t, seedDir, "config", "user.email", "update-test@example.invalid")
	runGit(t, seedDir, "commit", "--allow-empty", "--quiet", "-m", "0.25.0")
	runGit(t, seedDir, "tag", "0.25.0")
	runGit(t, seedDir, "remote", "add", "origin", originDir)
	runGit(t, seedDir, "push", "--quiet", "origin", "HEAD:refs/heads/main", "--tags")
	runGit(t, "", "clone", "--quiet", originDir, installDir)
	runGit(t, installDir, "fetch", "--quiet", "origin", "main")
	// A branch with the same short name must not supply the release notes.
	runGit(t, installDir, "branch", "0.25.1")
	head := runGit(t, installDir, "rev-parse", "HEAD")
	fetchHead, err := os.ReadFile(filepath.Join(installDir, ".git", "FETCH_HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	localFile := filepath.Join(installDir, "local-change")
	if err := os.WriteFile(localFile, []byte("staged"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, installDir, "add", "local-change")
	if err := os.WriteFile(localFile, []byte("unstaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	staged := runGit(t, installDir, "diff", "--cached")
	unstaged := runGit(t, installDir, "diff")
	notes := "## Fixes\n\n- Preserve project files.\n- Keep updates reliable."
	runGit(t, seedDir, "commit", "--allow-empty", "--quiet", "-m", "0.25.1", "-m", notes)
	runGit(t, seedDir, "tag", "0.25.1")
	runGit(t, seedDir, "tag", "-a", "0.25.2", "-m", "Tag annotation is not the release commit body")
	runGit(t, seedDir, "tag", "0.25.3")
	runGit(t, seedDir, "push", "--quiet", "origin", "HEAD:refs/heads/main", "--tags")

	for _, tag := range []string{"0.25.1", "0.25.2"} {
		got, err := (Client{}).ReadReleaseNotes(context.Background(), installDir, tag)
		if err != nil || got != notes {
			t.Fatalf("%s notes = %q, error = %v", tag, got, err)
		}
	}
	if err := exec.Command("git", "-C", installDir, "rev-parse", "--verify", "--quiet", "refs/tags/0.25.3").Run(); err == nil {
		t.Fatal("fetched an unrelated release tag")
	}
	if runGit(t, installDir, "rev-parse", "HEAD") != head || runGit(t, installDir, "diff", "--cached") != staged || runGit(t, installDir, "diff") != unstaged {
		t.Fatal("reading notes changed the checkout or index")
	}
	afterFetchHead, err := os.ReadFile(filepath.Join(installDir, ".git", "FETCH_HEAD"))
	if err != nil || string(afterFetchHead) != string(fetchHead) {
		t.Fatal("reading notes replaced FETCH_HEAD")
	}

	// Already fetched notes and releases without notes remain readable offline.
	runGit(t, installDir, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "unreachable"))
	for tag, want := range map[string]string{"0.25.0": "", "0.25.1": notes} {
		got, err := (Client{}).ReadReleaseNotes(context.Background(), installDir, tag)
		if err != nil || got != want {
			t.Fatalf("offline %s notes = %q, error = %v", tag, got, err)
		}
	}
	if _, err := (Client{}).ReadReleaseNotes(context.Background(), installDir, "0.99.0"); err == nil {
		t.Fatal("unavailable release was reported as having empty notes")
	}
}

func TestStartUpdaterSelectsReleaseScript(t *testing.T) {
	installDir := t.TempDir()
	infraDir := filepath.Join(installDir, "infra")
	if err := os.Mkdir(infraDir, 0o755); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(installDir, "selected")
	contents := "#!/usr/bin/env bash\nprintf '%s:%s:%s' 'deploy-app' \"$1\" \"$FUTRX_UPDATE_PROGRESS_PATH\" > \"$MARKER_PATH\"\n"
	if err := os.WriteFile(filepath.Join(infraDir, "deploy-app.sh"), []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MARKER_PATH", markerPath)

	logPath := filepath.Join(installDir, "run.log")
	donePath := filepath.Join(installDir, "done.json")
	progressPath := filepath.Join(installDir, "progress.json")
	if _, err := (Client{}).StartUpdater(serviceselfupdate.UpdaterLaunch{
		InstallDir:   installDir,
		Target:       "0.4.2",
		Kind:         serviceselfupdate.UpdateKindApplication,
		LogPath:      logPath,
		DonePath:     donePath,
		ProgressPath: progressPath,
	}); err != nil {
		t.Fatal(err)
	}
	waitForDone(t, donePath)

	selected, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(selected), "deploy-app:--ref=0.4.2:"+progressPath; got != want {
		t.Fatalf("selected script = %q, want %q", got, want)
	}
}

func TestStartUpdaterPreselectsInfrastructureRelease(t *testing.T) {
	originDir := filepath.Join(t.TempDir(), "origin.git")
	seedDir := filepath.Join(t.TempDir(), "seed")
	installDir := filepath.Join(t.TempDir(), "install")
	runGit(t, "", "init", "--bare", "--quiet", originDir)
	runGit(t, originDir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGit(t, "", "init", "--quiet", seedDir)
	runGit(t, seedDir, "config", "user.name", "Update Test")
	runGit(t, seedDir, "config", "user.email", "update-test@example.invalid")

	infraDir := filepath.Join(seedDir, "infra")
	if err := os.Mkdir(infraDir, 0o755); err != nil {
		t.Fatal(err)
	}
	staleScript := "#!/usr/bin/env bash\necho stale updater executed >&2\nexit 91\n"
	if err := os.WriteFile(filepath.Join(infraDir, "update.sh"), []byte(staleScript), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, seedDir, "add", "infra/update.sh")
	runGit(t, seedDir, "commit", "--quiet", "-m", "stale updater")
	runGit(t, seedDir, "tag", "0.16.0")
	runGit(t, seedDir, "remote", "add", "origin", originDir)
	runGit(t, seedDir, "push", "--quiet", "origin", "HEAD:refs/heads/main", "--tags")
	runGit(t, "", "clone", "--quiet", "--branch", "main", originDir, installDir)

	markerPath := filepath.Join(installDir, "selected")
	targetScript := "#!/usr/bin/env bash\nprintf '%s:%s:%s:%s:%s' 'target' \"$1\" \"$FUTRX_UPDATE_REEXECED\" \"$FUTRX_INSTALL_DIR\" \"$FUTRX_LEGACY_INSTALL_DIR\" > \"$MARKER_PATH\"\n"
	if err := os.WriteFile(filepath.Join(infraDir, "update.sh"), []byte(targetScript), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, seedDir, "add", "infra/update.sh")
	runGit(t, seedDir, "commit", "--quiet", "-m", "fixed updater")
	targetCommit := runGit(t, seedDir, "rev-parse", "HEAD")
	runGit(t, seedDir, "tag", "0.16.2")
	runGit(t, seedDir, "push", "--quiet", "origin", "HEAD:refs/heads/main", "--tags")
	t.Setenv("MARKER_PATH", markerPath)

	logPath := filepath.Join(installDir, "run.log")
	donePath := filepath.Join(installDir, "done.json")
	progressPath := filepath.Join(installDir, "progress.json")
	if _, err := (Client{}).StartUpdater(serviceselfupdate.UpdaterLaunch{
		InstallDir:   installDir,
		Target:       "0.16.2",
		Kind:         serviceselfupdate.UpdateKindInfrastructure,
		LogPath:      logPath,
		DonePath:     donePath,
		ProgressPath: progressPath,
	}); err != nil {
		t.Fatal(err)
	}
	waitForDone(t, donePath)

	selected, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "target:--ref=0.16.2:1:" + installDir + ":/opt/remote.futrx.dev"
	if got := string(selected); got != want {
		t.Fatalf("selected updater state = %q, want %q", got, want)
	}
	if got := runGit(t, installDir, "rev-parse", "HEAD"); got != targetCommit {
		t.Fatalf("selected checkout = %s, want %s", got, targetCommit)
	}
}

func TestStartUpdaterRejectsUnknownKind(t *testing.T) {
	dir := t.TempDir()
	if _, err := (Client{}).StartUpdater(serviceselfupdate.UpdaterLaunch{
		InstallDir:   dir,
		Target:       "0.4.2",
		Kind:         "surprise",
		LogPath:      filepath.Join(dir, "log"),
		DonePath:     filepath.Join(dir, "done"),
		ProgressPath: filepath.Join(dir, "progress"),
	}); err == nil {
		t.Fatal("StartUpdater accepted an unknown update kind")
	}
}

func waitForDone(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			var done struct {
				ExitCode int `json:"exitCode"`
			}
			if err := json.Unmarshal(data, &done); err != nil {
				t.Fatal(err)
			}
			if done.ExitCode != 0 {
				t.Fatalf("updater exited with %d", done.ExitCode)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
