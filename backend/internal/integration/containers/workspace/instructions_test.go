package workspace

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

const (
	claudeFile   = "/root/.claude/CLAUDE.md"
	codexFile    = "/root/.codex/AGENTS.md"
	sharedMarker = "/root/.claude/.agents-md.sha256"
)

// legacyFilesRunner fakes a container holding the instruction files an
// earlier release wrote: path -> content, plus symlinked paths and paths whose
// check fails. It answers retireScript the way the real shell does; the
// script itself is exercised against real files in
// TestRetireScriptAgainstRealFiles.
type legacyFilesRunner struct {
	files   map[string]string
	links   map[string]bool
	failing map[string]bool
	calls   []string
	removed []string
}

func (*legacyFilesRunner) Available() bool { return true }

func (r *legacyFilesRunner) RunStdin(_ context.Context, _ io.Reader, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	return "", nil
}

func (r *legacyFilesRunner) Run(_ context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	if len(args) < 4 || args[0] != "exec" || args[2] != "--" {
		return "", errors.New("unexpected command")
	}
	command := args[3:]
	switch command[0] {
	case "sh":
		path, marker := command[len(command)-2], command[len(command)-1]
		if r.failing[path] {
			return "", errors.New("exec failed")
		}
		content, exists := r.files[path]
		if !exists && !r.links[path] {
			return "absent\n", nil
		}
		if r.links[path] {
			return "kept\n", nil
		}
		want := strings.TrimSpace(r.files[marker])
		if want != "" && assets.Hash([]byte(content)) == want {
			delete(r.files, path)
			r.removed = append(r.removed, path)
			return "removed\n", nil
		}
		return "kept\n", nil
	case "rm":
		path := command[len(command)-1]
		if _, exists := r.files[path]; exists {
			delete(r.files, path)
			r.removed = append(r.removed, path)
		}
		return "", nil
	}
	return "", errors.New("unexpected command: " + strings.Join(command, " "))
}

func retireProvisioner(runner *legacyFilesRunner) *Provisioner {
	profiles := serviceprofiles.NewCatalog([]provisioning.Profile{
		{ID: "claude", LegacyInstructions: &provisioning.InstructionTarget{Path: claudeFile, HashPath: sharedMarker}},
		{ID: "codex", LegacyInstructions: &provisioning.InstructionTarget{Path: codexFile, HashPath: sharedMarker}},
		{ID: "kimi"},
	})
	return NewProvisioner(runner, profiles, assets.NewPublisher(runner), []byte("shared instructions"))
}

func TestRetireRemovesGeneratedCopiesAndTheirMarker(t *testing.T) {
	generated := "old generated instructions"
	runner := &legacyFilesRunner{files: map[string]string{
		claudeFile:   generated,
		codexFile:    generated,
		sharedMarker: assets.Hash([]byte(generated)),
	}}

	if err := retireProvisioner(runner).RetireHarnessInstructions(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{claudeFile, codexFile, sharedMarker} {
		if !slices.Contains(runner.removed, path) {
			t.Fatalf("%s was not removed; removed %v", path, runner.removed)
		}
	}
}

func TestRetireKeepsAnEditedCopyAndTheMarker(t *testing.T) {
	generated := "old generated instructions"
	runner := &legacyFilesRunner{files: map[string]string{
		claudeFile:   generated,
		codexFile:    generated + "\nmy own notes",
		sharedMarker: assets.Hash([]byte(generated)),
	}}

	if err := retireProvisioner(runner).RetireHarnessInstructions(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(runner.removed, []string{claudeFile}) {
		t.Fatalf("removed %v, want only the unedited Claude copy", runner.removed)
	}
}

func TestRetireKeepsFilesWithoutAMarker(t *testing.T) {
	runner := &legacyFilesRunner{files: map[string]string{claudeFile: "written by the user"}}

	if err := retireProvisioner(runner).RetireHarnessInstructions(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if len(runner.removed) != 0 {
		t.Fatalf("removed %v without a marker to prove the files were generated", runner.removed)
	}
}

func TestRetireLeavesSymlinksAlone(t *testing.T) {
	generated := "old generated instructions"
	runner := &legacyFilesRunner{
		files: map[string]string{
			claudeFile:   generated,
			sharedMarker: assets.Hash([]byte(generated)),
		},
		links: map[string]bool{claudeFile: true},
	}

	if err := retireProvisioner(runner).RetireHarnessInstructions(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(runner.removed, claudeFile) {
		t.Fatalf("a symlink was removed: %v", runner.removed)
	}
}

func TestRetireKeepsTheMarkerWhenACheckFails(t *testing.T) {
	generated := "old generated instructions"
	runner := &legacyFilesRunner{
		files: map[string]string{
			claudeFile:   generated,
			codexFile:    generated,
			sharedMarker: assets.Hash([]byte(generated)),
		},
		failing: map[string]bool{codexFile: true},
	}

	provisioner := retireProvisioner(runner)
	if err := provisioner.RetireHarnessInstructions(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(runner.removed, []string{claudeFile}) {
		t.Fatalf("removed %v, want only the Claude copy and no marker", runner.removed)
	}

	// The container is not recorded as clean, so the next run retries.
	delete(runner.failing, codexFile)
	if err := provisioner.RetireHarnessInstructions(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(runner.removed, []string{claudeFile, codexFile, sharedMarker}) {
		t.Fatalf("removed %v after the retry", runner.removed)
	}
}

func TestRetireRemovesAMarkerWhoseFilesAreAlreadyGone(t *testing.T) {
	runner := &legacyFilesRunner{files: map[string]string{sharedMarker: "abc"}}

	if err := retireProvisioner(runner).RetireHarnessInstructions(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(runner.removed, []string{sharedMarker}) {
		t.Fatalf("removed %v, want the orphaned marker", runner.removed)
	}
}

func TestRetireScriptAgainstRealFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("retireScript runs in Linux containers")
	}
	for _, tool := range []string{"sh", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	run := func(path string) string {
		out, err := exec.Command("sh", "-c", retireScript, "sh", path, marker).CombinedOutput()
		if err != nil {
			t.Fatalf("script failed: %v: %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	generated := []byte("generated\n")
	if err := os.WriteFile(marker, []byte(assets.Hash(generated)), 0o644); err != nil {
		t.Fatal(err)
	}

	unedited := filepath.Join(dir, "CLAUDE.md")
	edited := filepath.Join(dir, "AGENTS.md")
	link := filepath.Join(dir, "LINKED.md")
	if err := os.WriteFile(unedited, generated, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(edited, []byte("generated\nmine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(unedited, link); err != nil {
		t.Fatal(err)
	}

	if got := run(link); got != "kept" {
		t.Fatalf("symlink: %q, want kept", got)
	}
	if got := run(unedited); got != "removed" {
		t.Fatalf("unedited copy: %q, want removed", got)
	}
	if _, err := os.Stat(unedited); !os.IsNotExist(err) {
		t.Fatalf("unedited copy still exists: %v", err)
	}
	if got := run(edited); got != "kept" {
		t.Fatalf("edited copy: %q, want kept", got)
	}
	if content, err := os.ReadFile(edited); err != nil || string(content) != "generated\nmine\n" {
		t.Fatalf("edited copy was not restored intact: %q, %v", content, err)
	}
	if staged, _ := filepath.Glob(filepath.Join(dir, "*.remote-retire.*")); len(staged) != 0 {
		t.Fatalf("staging files left behind: %v", staged)
	}
	if got := run(filepath.Join(dir, "missing.md")); got != "absent" {
		t.Fatalf("missing file: %q, want absent", got)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if got := run(edited); got != "kept" {
		t.Fatalf("file without a marker: %q, want kept", got)
	}
}

func TestRetireIsANoOpOnACleanContainer(t *testing.T) {
	runner := &legacyFilesRunner{files: map[string]string{}}
	provisioner := retireProvisioner(runner)

	if err := provisioner.RetireHarnessInstructions(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	firstRun := len(runner.calls)
	if err := provisioner.RetireHarnessInstructions(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if len(runner.removed) != 0 {
		t.Fatalf("removed %v from a clean container", runner.removed)
	}
	if len(runner.calls) != firstRun {
		t.Fatalf("second run issued %d more commands after the container was confirmed clean", len(runner.calls)-firstRun)
	}
	if provisioner.HarnessInstructions() != "shared instructions" {
		t.Fatalf("instructions = %q", provisioner.HarnessInstructions())
	}
}
