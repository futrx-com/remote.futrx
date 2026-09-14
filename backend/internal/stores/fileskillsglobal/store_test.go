package fileskillsglobal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	serviceskills "github.com/futrx-com/remote.futrx.com/internal/service/skills"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dataDir := t.TempDir()
	store, err := New(dataDir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store, Dir(dataDir)
}

func save(t *testing.T, store *Store, name string, files map[string][]byte, alwaysOn bool) serviceskills.GlobalRecord {
	t.Helper()
	record, err := store.Save(context.Background(), serviceskills.GlobalRecord{
		Name:     name,
		Files:    files,
		AlwaysOn: alwaysOn,
	})
	if err != nil {
		t.Fatalf("save %s: %v", name, err)
	}
	return record
}

func TestStoreSaveGetListDelete(t *testing.T) {
	store, root := newStore(t)
	ctx := context.Background()

	saved := save(t, store, "code-review-guard", map[string][]byte{
		"SKILL.md":            []byte("---\nname: code-review-guard\n---\n"),
		"references/rules.md": []byte("more rules"),
	}, true)

	if saved.UpdatedAt == 0 {
		t.Fatal("save should stamp updatedAt")
	}
	if got, want := len(saved.FileNames), 2; got != want {
		t.Fatalf("file names = %v, want %d entries", saved.FileNames, want)
	}
	if _, err := os.Stat(filepath.Join(root, "code-review-guard", "SKILL.md")); err != nil {
		t.Fatalf("SKILL.md is not on disk: %v", err)
	}

	got, err := store.Get(ctx, "code-review-guard")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got.Files["references/rules.md"]) != "more rules" {
		t.Fatalf("supporting file content = %q", got.Files["references/rules.md"])
	}
	if !got.AlwaysOn {
		t.Fatal("alwaysOn flag was not persisted")
	}

	save(t, store, "wordpress-guard", map[string][]byte{"SKILL.md": []byte("# wp")}, false)
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Name != "code-review-guard" || list[1].Name != "wordpress-guard" {
		t.Fatalf("list = %#v, want both skills sorted by name", list)
	}
	if list[0].Files != nil {
		t.Fatal("list should not load file contents")
	}

	if err := store.Delete(ctx, "code-review-guard"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(ctx, "code-review-guard"); !errors.Is(err, serviceskills.ErrGlobalSkillNotFound) {
		t.Fatalf("get after delete = %v, want ErrGlobalSkillNotFound", err)
	}
	if _, err := os.Stat(filepath.Join(root, "code-review-guard")); !os.IsNotExist(err) {
		t.Fatalf("skill directory survived delete: %v", err)
	}
}

func TestStoreSaveReplacesPreviousFiles(t *testing.T) {
	store, root := newStore(t)

	save(t, store, "guard", map[string][]byte{
		"SKILL.md":   []byte("v1"),
		"extra/a.md": []byte("stale"),
	}, false)
	replaced := save(t, store, "guard", map[string][]byte{"SKILL.md": []byte("v2")}, false)

	if len(replaced.FileNames) != 1 || replaced.FileNames[0] != "SKILL.md" {
		t.Fatalf("file names after replace = %v, want only SKILL.md", replaced.FileNames)
	}
	if _, err := os.Stat(filepath.Join(root, "guard", "extra", "a.md")); !os.IsNotExist(err) {
		t.Fatalf("removed file survived replace: %v", err)
	}
}

func TestStoreAlwaysOnPersistsInIndexFile(t *testing.T) {
	store, root := newStore(t)
	ctx := context.Background()

	save(t, store, "guard", map[string][]byte{"SKILL.md": []byte("# guard")}, false)
	if _, err := store.SetAlwaysOn(ctx, "guard", true); err != nil {
		t.Fatalf("set always on: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, indexFileName))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var index indexFile
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatalf("parse index: %v", err)
	}
	if !index.Skills["guard"].AlwaysOn {
		t.Fatalf("index = %s, want guard alwaysOn", raw)
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || !list[0].AlwaysOn {
		t.Fatalf("list = %#v, want alwaysOn restored from the index", list)
	}

	if err := store.Delete(ctx, "guard"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	raw, err = os.ReadFile(filepath.Join(root, indexFileName))
	if err != nil {
		t.Fatalf("read index after delete: %v", err)
	}
	index = indexFile{}
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatalf("parse index after delete: %v", err)
	}
	if _, ok := index.Skills["guard"]; ok {
		t.Fatalf("index kept a deleted skill: %s", raw)
	}
}

func TestStoreIgnoresReservedAndIncompleteEntries(t *testing.T) {
	store, root := newStore(t)

	for _, directory := range []string{"_reserved", ".hidden", "no-manifest", "Upper-Case"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
	}
	for _, directory := range []string{"_reserved", ".hidden", "Upper-Case"} {
		if err := os.WriteFile(filepath.Join(root, directory, "SKILL.md"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write manifest in %s: %v", directory, err)
		}
	}
	save(t, store, "real", map[string][]byte{"SKILL.md": []byte("# real")}, false)

	list, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "real" {
		t.Fatalf("list = %#v, want only the valid skill", list)
	}
}

func TestStoreRejectsInvalidNames(t *testing.T) {
	store, _ := newStore(t)
	ctx := context.Background()

	for _, name := range []string{"", "../escape", "Upper", "with space", "_reserved", ".hidden"} {
		if _, err := store.Save(ctx, serviceskills.GlobalRecord{
			Name:  name,
			Files: map[string][]byte{"SKILL.md": []byte("x")},
		}); !errors.Is(err, serviceskills.ErrInvalidGlobalSkillName) {
			t.Fatalf("save %q = %v, want ErrInvalidGlobalSkillName", name, err)
		}
		if err := store.Delete(ctx, name); !errors.Is(err, serviceskills.ErrInvalidGlobalSkillName) {
			t.Fatalf("delete %q = %v, want ErrInvalidGlobalSkillName", name, err)
		}
	}
}

func TestStoreMissingSkillErrors(t *testing.T) {
	store, _ := newStore(t)
	ctx := context.Background()

	if _, err := store.Get(ctx, "absent"); !errors.Is(err, serviceskills.ErrGlobalSkillNotFound) {
		t.Fatalf("get = %v, want ErrGlobalSkillNotFound", err)
	}
	if _, err := store.SetAlwaysOn(ctx, "absent", true); !errors.Is(err, serviceskills.ErrGlobalSkillNotFound) {
		t.Fatalf("set always on = %v, want ErrGlobalSkillNotFound", err)
	}
	if err := store.Delete(ctx, "absent"); !errors.Is(err, serviceskills.ErrGlobalSkillNotFound) {
		t.Fatalf("delete = %v, want ErrGlobalSkillNotFound", err)
	}
}
