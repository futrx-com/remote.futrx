package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// memoryGlobalRepository is an in-memory stand-in for the file-backed
// library. It mirrors the store contract: List omits file contents.
type memoryGlobalRepository struct {
	records map[string]GlobalRecord
	saves   int
}

func newMemoryGlobalRepository() *memoryGlobalRepository {
	return &memoryGlobalRepository{records: map[string]GlobalRecord{}}
}

func (r *memoryGlobalRepository) List(context.Context) ([]GlobalRecord, error) {
	names := make([]string, 0, len(r.records))
	for name := range r.records {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]GlobalRecord, 0, len(names))
	for _, name := range names {
		record := r.records[name]
		record.Files = nil
		out = append(out, record)
	}
	return out, nil
}

func (r *memoryGlobalRepository) Get(_ context.Context, name string) (GlobalRecord, error) {
	record, ok := r.records[name]
	if !ok {
		return GlobalRecord{}, ErrGlobalSkillNotFound
	}
	return record, nil
}

func (r *memoryGlobalRepository) Save(_ context.Context, record GlobalRecord) (GlobalRecord, error) {
	names := make([]string, 0, len(record.Files))
	for name := range record.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	record.FileNames = names
	r.records[record.Name] = record
	r.saves++
	return record, nil
}

func (r *memoryGlobalRepository) SetAlwaysOn(_ context.Context, name string, alwaysOn bool) (GlobalRecord, error) {
	record, ok := r.records[name]
	if !ok {
		return GlobalRecord{}, ErrGlobalSkillNotFound
	}
	record.AlwaysOn = alwaysOn
	r.records[name] = record
	return record, nil
}

func (r *memoryGlobalRepository) Delete(_ context.Context, name string) error {
	if _, ok := r.records[name]; !ok {
		return ErrGlobalSkillNotFound
	}
	delete(r.records, name)
	return nil
}

type stubProjectCatalog struct {
	project serviceproject.Meta
	err     error
}

func (s stubProjectCatalog) Get(context.Context, serviceproject.ID) (serviceproject.Meta, error) {
	return s.project, s.err
}

func (s stubProjectCatalog) HasAccess(context.Context, serviceproject.ID, string) (bool, error) {
	return true, nil
}

type stubAuthorizer struct {
	email string
	admin bool
}

func (s stubAuthorizer) CurrentSession(string) (*serviceauth.Session, error) {
	return &serviceauth.Session{Email: s.email}, nil
}

func (s stubAuthorizer) IsAdmin(context.Context, string) (bool, error) {
	return s.admin, nil
}

const sampleManifest = "---\nname: Code Review Guard\ndescription: Review checklist.\n---\n\nBody\n"

func TestValidGlobalSkillName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "simple", input: "code-review-guard", want: true},
		{name: "digits and dots", input: "guard.v2_1", want: true},
		{name: "empty", input: "", want: false},
		{name: "uppercase", input: "Guard", want: false},
		{name: "space", input: "code guard", want: false},
		{name: "traversal", input: "../etc", want: false},
		{name: "leading dot", input: ".hidden", want: false},
		{name: "reserved prefix", input: "_index", want: false},
		{name: "max length", input: strings.Repeat("a", MaxGlobalSkillNameLength), want: true},
		{name: "too long", input: strings.Repeat("a", MaxGlobalSkillNameLength+1), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ValidGlobalSkillName(test.input); got != test.want {
				t.Fatalf("ValidGlobalSkillName(%q) = %t, want %t", test.input, got, test.want)
			}
		})
	}
}

func TestNormalizeGlobalSkillFiles(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  error
	}{
		{name: "manifest only", files: map[string]string{"SKILL.md": "x"}},
		{name: "nested support file", files: map[string]string{"SKILL.md": "x", "references/a.md": "y"}},
		{name: "no files", files: nil, want: ErrMissingSkillManifest},
		{name: "missing manifest", files: map[string]string{"README.md": "x"}, want: ErrMissingSkillManifest},
		{name: "absolute path", files: map[string]string{"SKILL.md": "x", "/etc/passwd": "y"}, want: ErrInvalidGlobalSkillFile},
		{name: "parent traversal", files: map[string]string{"SKILL.md": "x", "../escape.md": "y"}, want: ErrInvalidGlobalSkillFile},
		{name: "hidden segment", files: map[string]string{"SKILL.md": "x", ".git/config": "y"}, want: ErrInvalidGlobalSkillFile},
		{name: "windows separator", files: map[string]string{"SKILL.md": "x", "refs\\a.md": "y"}},
		{name: "oversized file", files: map[string]string{"SKILL.md": string(make([]byte, MaxGlobalSkillFileBytes+1))}, want: ErrGlobalSkillTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeGlobalSkillFiles(test.files)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if test.want == nil && len(got) != len(test.files) {
				t.Fatalf("normalized %d files, want %d", len(got), len(test.files))
			}
		})
	}
}

func TestGlobalServiceCreateRejectsDuplicates(t *testing.T) {
	service := NewGlobalService(newMemoryGlobalRepository(), nil)
	ctx := context.Background()

	if _, err := service.Create(ctx, GlobalInput{Name: "guard", Files: map[string]string{"SKILL.md": sampleManifest}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err := service.Create(ctx, GlobalInput{Name: "guard", Files: map[string]string{"SKILL.md": sampleManifest}})
	if !errors.Is(err, ErrGlobalSkillExists) {
		t.Fatalf("duplicate create = %v, want ErrGlobalSkillExists", err)
	}
}

func TestGlobalServiceUpdateFlagOnlyKeepsFiles(t *testing.T) {
	repo := newMemoryGlobalRepository()
	service := NewGlobalService(repo, nil)
	ctx := context.Background()

	if _, err := service.Create(ctx, GlobalInput{
		Name:  "guard",
		Files: map[string]string{"SKILL.md": sampleManifest, "refs/a.md": "a"},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	alwaysOn := true
	updated, err := service.Update(ctx, "guard", GlobalUpdate{AlwaysOn: &alwaysOn})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !updated.AlwaysOn {
		t.Fatal("update should have enabled alwaysOn")
	}
	if repo.saves != 1 {
		t.Fatalf("saves = %d, want 1 (a flag change must not rewrite files)", repo.saves)
	}
	stored, err := service.Get(ctx, "guard")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(stored.Files) != 2 {
		t.Fatalf("files after flag update = %v, want both preserved", stored.FileNames)
	}
}

func TestGlobalServiceDescribesManifestMetadata(t *testing.T) {
	service := NewGlobalService(newMemoryGlobalRepository(), nil)
	ctx := context.Background()

	created, err := service.Create(ctx, GlobalInput{
		Name:  "code-review-guard",
		Files: map[string]string{"SKILL.md": sampleManifest},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Title != "Code Review Guard" || created.Description != "Review checklist." {
		t.Fatalf("metadata = %#v, want the SKILL.md frontmatter", created)
	}
}

func TestCatalogEntriesFlagScopeAndShadowing(t *testing.T) {
	repo := newMemoryGlobalRepository()
	service := NewGlobalService(repo, nil)
	ctx := context.Background()

	for _, name := range []string{"code-review-guard", "wordpress-guard"} {
		if _, err := service.Create(ctx, GlobalInput{Name: name, Files: map[string]string{"SKILL.md": sampleManifest}}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	if _, err := service.Update(ctx, "code-review-guard", GlobalUpdate{AlwaysOn: boolPointer(true)}); err != nil {
		t.Fatalf("mark always on: %v", err)
	}

	existing := []Skill{
		{Name: "Wordpress Guard", Command: "wordpress-guard", Source: "project"},
		{Name: "Host Only", Command: "code-review-guard", Source: "user"},
	}
	entries := service.CatalogEntries(ctx, ProviderClaude, existing)
	if len(entries) != 2 {
		t.Fatalf("entries = %#v, want both global skills", entries)
	}

	byCommand := map[string]Skill{}
	for _, entry := range entries {
		byCommand[entry.Command] = entry
	}
	shadowed := byCommand["wordpress-guard"]
	if !shadowed.Shadowed {
		t.Fatal("a project skill of the same name must shadow the global one")
	}
	visible := byCommand["code-review-guard"]
	if visible.Shadowed {
		t.Fatal("a host-level user skill must not shadow a global skill: it is not in the container")
	}
	if visible.Scope != ScopeGlobal || visible.Source != SourceGlobal || !visible.ReadOnly {
		t.Fatalf("global entry = %#v, want global scope, global source, read only", visible)
	}
	if !visible.AlwaysOn {
		t.Fatal("alwaysOn flag should reach the picker")
	}
	if visible.Provider != ProviderClaude {
		t.Fatalf("provider = %q, want the requested provider", visible.Provider)
	}
	if visible.Name != "Code Review Guard" {
		t.Fatalf("name = %q, want the SKILL.md name", visible.Name)
	}
}

func TestCatalogMergesGlobalSkillsForProjectChatsOnly(t *testing.T) {
	workspace := t.TempDir()
	writeSkill(t, filepath.Join(workspace, ".agents", "skills", "local-only", "SKILL.md"), "# Local Only")

	repo := newMemoryGlobalRepository()
	global := NewGlobalService(repo, nil)
	if _, err := global.Create(context.Background(), GlobalInput{
		Name:  "code-review-guard",
		Files: map[string]string{"SKILL.md": sampleManifest},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	skills := NewWithSkillHomes(t.TempDir(), t.TempDir(), t.TempDir())
	catalog := NewCatalog(
		skills,
		stubProjectCatalog{project: serviceproject.Meta{ID: "p1", Cwd: workspace}},
		stubAuthorizer{email: "admin@example.test", admin: true},
	).WithGlobalLibrary(global)

	projectSkills, err := catalog.List(context.Background(), ListQuery{Provider: ProviderClaude, ProjectID: "p1"})
	if err != nil {
		t.Fatalf("list project skills: %v", err)
	}
	if !hasSkillCommand(projectSkills, "code-review-guard") {
		t.Fatalf("project listing = %#v, want the global skill merged in", projectSkills)
	}
	if !hasSkillCommand(projectSkills, "local-only") {
		t.Fatalf("project listing = %#v, want the project skill kept", projectSkills)
	}

	looseSkills, err := catalog.List(context.Background(), ListQuery{Provider: ProviderClaude})
	if err != nil {
		t.Fatalf("list loose skills: %v", err)
	}
	if hasSkillCommand(looseSkills, "code-review-guard") {
		t.Fatalf("loose listing = %#v, want no global skills (there is no container)", looseSkills)
	}
}

func TestDefaultSkillsReturnsOnlyAlwaysOnEntries(t *testing.T) {
	service := NewGlobalService(newMemoryGlobalRepository(), nil)
	ctx := context.Background()

	for _, name := range []string{"pinned", "optional"} {
		if _, err := service.Create(ctx, GlobalInput{Name: name, Files: map[string]string{"SKILL.md": sampleManifest}}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	if _, err := service.Update(ctx, "pinned", GlobalUpdate{AlwaysOn: boolPointer(true)}); err != nil {
		t.Fatalf("mark always on: %v", err)
	}

	defaults, err := service.DefaultSkills(ctx, ProviderCodex)
	if err != nil {
		t.Fatalf("default skills: %v", err)
	}
	if len(defaults) != 1 || defaults[0].Command != "pinned" {
		t.Fatalf("defaults = %#v, want only the pinned skill", defaults)
	}
	if defaults[0].Source != SourceGlobal || defaults[0].Provider != ProviderCodex {
		t.Fatalf("default = %#v, want global source and the chat provider", defaults[0])
	}
}

func TestImportFromProjectCopiesSkillDirectory(t *testing.T) {
	workspace := t.TempDir()
	writeSkill(t, filepath.Join(workspace, ".agents", "skills", "team-style", "SKILL.md"), sampleManifest)
	if err := os.MkdirAll(filepath.Join(workspace, ".agents", "skills", "team-style", "refs"), 0o755); err != nil {
		t.Fatalf("create refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".agents", "skills", "team-style", "refs", "a.md"), []byte("ref"), 0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}

	service := NewGlobalService(
		newMemoryGlobalRepository(),
		stubProjectCatalog{project: serviceproject.Meta{ID: "p1", Cwd: workspace}},
	)
	ctx := context.Background()

	imported, err := service.ImportFromProject(ctx, "p1", "team-style", "", true)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.Name != "team-style" || !imported.AlwaysOn {
		t.Fatalf("imported = %#v, want the source name and the always-on flag", imported)
	}
	if imported.Files["refs/a.md"] != "ref" {
		t.Fatalf("supporting files = %v, want refs/a.md copied", imported.FileNames)
	}

	if _, err := service.ImportFromProject(ctx, "p1", "team-style", "", false); !errors.Is(err, ErrGlobalSkillExists) {
		t.Fatalf("second import = %v, want ErrGlobalSkillExists", err)
	}
	if _, err := service.ImportFromProject(ctx, "p1", "absent", "", false); !errors.Is(err, ErrProjectSkillNotFound) {
		t.Fatalf("import of a missing skill = %v, want ErrProjectSkillNotFound", err)
	}
}

func TestSeedBuiltinsInstallsOnceAndNeverOverwrites(t *testing.T) {
	repo := newMemoryGlobalRepository()
	service := NewGlobalService(repo, nil)
	ctx := context.Background()

	installed, err := service.SeedBuiltins(ctx)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if installed != 2 {
		t.Fatalf("installed %d built-ins, want 2", installed)
	}
	for _, name := range []string{"code-review-guard", "wordpress-guard"} {
		skill, err := service.Get(ctx, name)
		if err != nil {
			t.Fatalf("get %s: %v", name, err)
		}
		if skill.Title == "" || skill.Description == "" {
			t.Fatalf("built-in %s = %#v, want frontmatter name and description", name, skill)
		}
	}

	edited := repo.records["code-review-guard"]
	edited.Files = map[string][]byte{"SKILL.md": []byte("---\nname: mine\n---\n")}
	repo.records["code-review-guard"] = edited

	again, err := service.SeedBuiltins(ctx)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if again != 0 {
		t.Fatalf("second seed installed %d skills, want 0", again)
	}
	if string(repo.records["code-review-guard"].Files["SKILL.md"]) != "---\nname: mine\n---\n" {
		t.Fatal("seeding overwrote an operator edit")
	}
}

func TestSeedBuiltinsRefillsAnEmptiedLibrary(t *testing.T) {
	repo := newMemoryGlobalRepository()
	service := NewGlobalService(repo, nil)
	ctx := context.Background()

	if _, err := service.SeedBuiltins(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for name := range repo.records {
		if err := service.Delete(ctx, name); err != nil {
			t.Fatalf("delete %s: %v", name, err)
		}
	}
	installed, err := service.SeedBuiltins(ctx)
	if err != nil {
		t.Fatalf("reseed: %v", err)
	}
	if installed != 2 {
		t.Fatalf("reseed installed %d skills, want 2", installed)
	}
}

func TestBuiltinGlobalSkillsAreValid(t *testing.T) {
	seeds, err := BuiltinGlobalSkills()
	if err != nil {
		t.Fatalf("built-ins: %v", err)
	}
	if len(seeds) != 2 {
		t.Fatalf("built-ins = %d, want 2", len(seeds))
	}
	for _, seed := range seeds {
		if !ValidGlobalSkillName(seed.Name) {
			t.Fatalf("built-in name %q is not a valid skill directory name", seed.Name)
		}
		if _, err := NormalizeGlobalSkillFiles(seed.Files); err != nil {
			t.Fatalf("built-in %s files: %v", seed.Name, err)
		}
		metadata := parseSkillMetadata([]byte(seed.Files[SkillFileName]))
		if metadata.Name != seed.Name {
			t.Fatalf("built-in %s frontmatter name = %q, want the directory name", seed.Name, metadata.Name)
		}
		if metadata.Description == "" {
			t.Fatalf("built-in %s has no frontmatter description", seed.Name)
		}
	}
}

func boolPointer(value bool) *bool {
	return &value
}
