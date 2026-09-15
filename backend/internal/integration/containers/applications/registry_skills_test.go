package applications

import (
	"testing"
	"testing/fstest"
)

// skillCatalog builds a one-application catalog carrying the given extra files.
func skillCatalog(extra map[string]string) (*Registry, error) {
	fsys := fstest.MapFS{
		"applications/kit/application.json": &fstest.MapFile{Data: []byte(`{
			"name": "Kit",
			"version": "1.0.0",
			"scopes": ["project"],
			"service": "kit"
		}`)},
		"applications/kit/infra/install.sh": &fstest.MapFile{Data: []byte("#!/usr/bin/env bash\n")},
	}
	for name, body := range extra {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return NewRegistryFromFS(fsys)
}

// A skills/ directory opts an application in, the same way ui/ does.
func TestApplicationShipsTheSkillsInItsSkillsDirectory(t *testing.T) {
	r, err := skillCatalog(map[string]string{
		"applications/kit/skills/mount-bucket/SKILL.md": "# using the mounted bucket\n",
		"applications/kit/skills/tune-cache/SKILL.md":   "# cache tuning\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	application, ok := r.Get("kit")
	if !ok {
		t.Fatal("application did not load")
	}
	if len(application.Skills) != 2 || application.Skills[0] != "mount-bucket" || application.Skills[1] != "tune-cache" {
		t.Fatalf("skills not discovered in a stable order: %v", application.Skills)
	}
	body, ok := r.Skill("kit", "mount-bucket")
	if !ok || string(body) != "# using the mounted bucket\n" {
		t.Fatalf("skill body not readable: %q %v", body, ok)
	}
	if _, ok = r.Skill("kit", "never-shipped"); ok {
		t.Fatal("a name the application does not ship was readable")
	}
}

func TestApplicationWithoutSkillsShipsNone(t *testing.T) {
	r, err := skillCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	application, _ := r.Get("kit")
	if len(application.Skills) != 0 {
		t.Fatalf("invented skills: %v", application.Skills)
	}
}

// A directory of loose files is not a skill, and saying so at load time beats
// publishing something the agent cannot read.
func TestSkillDirectoryWithoutASkillFileIsRejected(t *testing.T) {
	_, err := skillCatalog(map[string]string{"applications/kit/skills/mount-bucket/notes.txt": "stray\n"})
	if err == nil {
		t.Fatal("accepted a skill directory with no SKILL.md")
	}
}

// Skill names become directories in a project workspace.
func TestSkillNameMustBeOnePathComponent(t *testing.T) {
	_, err := skillCatalog(map[string]string{"applications/kit/skills/Mount Bucket/SKILL.md": "# no\n"})
	if err == nil {
		t.Fatal("accepted a skill name that is not a safe path component")
	}
}
