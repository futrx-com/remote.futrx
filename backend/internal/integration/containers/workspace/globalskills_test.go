package workspace

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
)

// stubRunner records every lxc invocation and answers marker reads from a
// map, standing in for a container that already holds some published state.
type stubRunner struct {
	unavailable bool
	markers     map[string]string
	calls       []string
	pushes      []string
	scripts     []string
}

func (r *stubRunner) Available() bool { return !r.unavailable }

func (r *stubRunner) Run(_ context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	switch {
	case len(args) >= 5 && args[0] == "exec" && args[3] == "cat":
		return r.markers[args[4]], nil
	case len(args) >= 5 && args[0] == "exec" && args[3] == "sh":
		r.scripts = append(r.scripts, args[len(args)-1])
		return "", nil
	case len(args) >= 2 && args[0] == "file" && args[1] == "push":
		r.pushes = append(r.pushes, args[len(args)-1])
		return "", nil
	}
	return "", nil
}

func (r *stubRunner) RunStdin(_ context.Context, _ io.Reader, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	return "", nil
}

func newTestProvisioner(runner *stubRunner, libraryDir string) *Provisioner {
	profiles := serviceprofiles.NewCatalog([]provisioning.Profile{
		{WorkspaceSkills: &provisioning.WorkspaceSkills{
			WorkspaceHome: "/workspace/.claude",
			HomeSkillsDir: "/root/.claude/skills",
		}},
	})
	return NewProvisioner(
		runner,
		profiles,
		assets.NewPublisher(runner),
		[]byte("instructions"),
		WithGlobalSkillLibrary(libraryDir),
	)
}

func writeLibrarySkill(t *testing.T, root, name string, files map[string]string) {
	t.Helper()
	for relative, content := range files {
		destination := filepath.Join(root, name, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			t.Fatalf("create %s: %v", destination, err)
		}
		if err := os.WriteFile(destination, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", destination, err)
		}
	}
}

func TestEnsureGlobalSkillsPublishesAndLinksLibrary(t *testing.T) {
	library := t.TempDir()
	writeLibrarySkill(t, library, "code-review-guard", map[string]string{
		"SKILL.md":            "---\nname: code-review-guard\n---\n",
		"references/rules.md": "rules",
	})
	writeLibrarySkill(t, library, "wordpress-guard", map[string]string{"SKILL.md": "# wp"})

	runner := &stubRunner{markers: map[string]string{}}
	if err := newTestProvisioner(runner, library).EnsureGlobalSkills(context.Background(), "project-1"); err != nil {
		t.Fatalf("ensure global skills: %v", err)
	}

	wantPushes := []string{
		"project-1/workspace/.agents/skills-global/code-review-guard/SKILL.md",
		"project-1/workspace/.agents/skills-global/code-review-guard/references/rules.md",
		"project-1/workspace/.agents/skills-global/wordpress-guard/SKILL.md",
	}
	if len(runner.pushes) != len(wantPushes) {
		t.Fatalf("pushed %v, want %v", runner.pushes, wantPushes)
	}
	for _, want := range wantPushes {
		found := false
		for _, got := range runner.pushes {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("pushed %v, missing %s", runner.pushes, want)
		}
	}

	if len(runner.scripts) != 1 {
		t.Fatalf("ran %d sync scripts, want 1", len(runner.scripts))
	}
	script := runner.scripts[0]
	for _, want := range []string{
		"link_global_skill 'code-review-guard'",
		"link_global_skill 'wordpress-guard'",
		"'code-review-guard'|'wordpress-guard')",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("sync script is missing %q:\n%s", want, script)
		}
	}

	directoryCall := ""
	for _, call := range runner.calls {
		if strings.Contains(call, "install -d") {
			directoryCall = call
		}
	}
	for _, want := range []string{
		"/workspace/.agents/skills-global",
		"/workspace/.agents/skills-global/code-review-guard/references",
	} {
		if !strings.Contains(directoryCall, want) {
			t.Fatalf("install -d call %q is missing %s", directoryCall, want)
		}
	}
}

func TestEnsureGlobalSkillsSkipsWhenMarkerMatches(t *testing.T) {
	library := t.TempDir()
	writeLibrarySkill(t, library, "guard", map[string]string{"SKILL.md": "# guard"})

	loaded, err := loadGlobalSkillLibrary(library)
	if err != nil {
		t.Fatalf("load library: %v", err)
	}
	runner := &stubRunner{markers: map[string]string{containerGlobalMarker: globalLibraryHash(loaded)}}
	if err := newTestProvisioner(runner, library).EnsureGlobalSkills(context.Background(), "project-1"); err != nil {
		t.Fatalf("ensure global skills: %v", err)
	}
	if len(runner.pushes) != 0 || len(runner.scripts) != 0 {
		t.Fatalf("converged an unchanged library: pushes=%v scripts=%d", runner.pushes, len(runner.scripts))
	}
	if len(runner.calls) != 1 {
		t.Fatalf("made %d calls, want a single marker read: %v", len(runner.calls), runner.calls)
	}
}

func TestEnsureGlobalSkillsPrunesEverythingForAnEmptyLibrary(t *testing.T) {
	runner := &stubRunner{markers: map[string]string{containerGlobalMarker: "stale"}}
	if err := newTestProvisioner(runner, t.TempDir()).EnsureGlobalSkills(context.Background(), "project-1"); err != nil {
		t.Fatalf("ensure global skills: %v", err)
	}
	if len(runner.pushes) != 0 {
		t.Fatalf("pushed %v for an empty library", runner.pushes)
	}
	if len(runner.scripts) != 1 {
		t.Fatalf("ran %d sync scripts, want 1", len(runner.scripts))
	}
	script := runner.scripts[0]
	if !strings.Contains(script, "''") {
		t.Fatalf("empty library should keep nothing:\n%s", script)
	}
	if strings.Contains(script, "link_global_skill ") {
		t.Fatalf("empty library should link nothing:\n%s", script)
	}
}

func TestEnsureGlobalSkillsIsDisabledWithoutALibraryDirectory(t *testing.T) {
	runner := &stubRunner{markers: map[string]string{}}
	if err := newTestProvisioner(runner, "").EnsureGlobalSkills(context.Background(), "project-1"); err != nil {
		t.Fatalf("ensure global skills: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("touched the container with no library configured: %v", runner.calls)
	}
}

func TestLoadGlobalSkillLibrarySkipsReservedEntries(t *testing.T) {
	library := t.TempDir()
	writeLibrarySkill(t, library, "guard", map[string]string{
		"SKILL.md":      "# guard",
		".skill.sha256": "marker",
		".git/HEAD":     "ref",
		"refs/notes.md": "notes",
	})
	writeLibrarySkill(t, library, "_reserved", map[string]string{"SKILL.md": "# nope"})
	writeLibrarySkill(t, library, "no-manifest", map[string]string{"README.md": "# nope"})
	if err := os.WriteFile(filepath.Join(library, "_index.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	loaded, err := loadGlobalSkillLibrary(library)
	if err != nil {
		t.Fatalf("load library: %v", err)
	}
	if len(loaded) != 1 || loaded[0].name != "guard" {
		t.Fatalf("library = %#v, want only the valid skill", loaded)
	}
	var paths []string
	for _, file := range loaded[0].files {
		paths = append(paths, file.path)
	}
	if strings.Join(paths, ",") != "SKILL.md,refs/notes.md" {
		t.Fatalf("files = %v, want the manifest and its support file only", paths)
	}
}

func TestGlobalLibraryHashTracksRenamesAndContent(t *testing.T) {
	base := []globalSkill{{name: "guard", files: []globalSkillFile{{path: "SKILL.md", content: []byte("a")}}}}
	renamed := []globalSkill{{name: "guard2", files: []globalSkillFile{{path: "SKILL.md", content: []byte("a")}}}}
	edited := []globalSkill{{name: "guard", files: []globalSkillFile{{path: "SKILL.md", content: []byte("b")}}}}

	if globalLibraryHash(base) == globalLibraryHash(renamed) {
		t.Fatal("a rename must change the library hash")
	}
	if globalLibraryHash(base) == globalLibraryHash(edited) {
		t.Fatal("an edit must change the library hash")
	}
	if globalLibraryHash(base) != globalLibraryHash(base) {
		t.Fatal("the library hash must be stable")
	}
}

func TestEnsureSkillLinksConvergesGlobalSkillsFirst(t *testing.T) {
	library := t.TempDir()
	writeLibrarySkill(t, library, "guard", map[string]string{"SKILL.md": "# guard"})

	runner := &stubRunner{markers: map[string]string{}}
	if err := newTestProvisioner(runner, library).EnsureSkillLinks(context.Background(), "project-1"); err != nil {
		t.Fatalf("ensure skill links: %v", err)
	}
	if len(runner.scripts) != 2 {
		t.Fatalf("ran %d scripts, want the global sync and the local link script", len(runner.scripts))
	}
	if !strings.Contains(runner.scripts[0], "link_global_skill") {
		t.Fatalf("global sync must run first:\n%s", runner.scripts[0])
	}
	if !strings.Contains(runner.scripts[1], "mirror_home_skills") {
		t.Fatalf("local link script must run second:\n%s", runner.scripts[1])
	}
}
