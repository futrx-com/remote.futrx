package applications

import (
	"testing"
	"testing/fstest"
)

// skillCatalog builds a one-image catalog carrying the given extra files.
func skillCatalog(extra map[string]string) (*Registry, error) {
	fsys := fstest.MapFS{
		"images/kit/image.json": &fstest.MapFile{Data: []byte(`{
			"name": "Kit",
			"version": "1.0.0",
			"type": "tool",
			"scopes": ["project"],
			"service": "kit"
		}`)},
		"images/kit/install.sh": &fstest.MapFile{Data: []byte("#!/usr/bin/env bash\n")},
	}
	for name, body := range extra {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return NewRegistryFromFS(fsys)
}

// A skills/ directory opts an image in, the same way ui/ does.
func TestImageShipsTheSkillsInItsSkillsDirectory(t *testing.T) {
	r, err := skillCatalog(map[string]string{
		"images/kit/skills/mount-bucket/SKILL.md": "# using the mounted bucket\n",
		"images/kit/skills/tune-cache/SKILL.md":   "# cache tuning\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	img, ok := r.Get("kit")
	if !ok {
		t.Fatal("image did not load")
	}
	if len(img.Skills) != 2 || img.Skills[0] != "mount-bucket" || img.Skills[1] != "tune-cache" {
		t.Fatalf("skills not discovered in a stable order: %v", img.Skills)
	}
	body, ok := r.Skill("kit", "mount-bucket")
	if !ok || string(body) != "# using the mounted bucket\n" {
		t.Fatalf("skill body not readable: %q %v", body, ok)
	}
	if _, ok = r.Skill("kit", "never-shipped"); ok {
		t.Fatal("a name the image does not ship was readable")
	}
}

func TestImageWithoutSkillsShipsNone(t *testing.T) {
	r, err := skillCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	img, _ := r.Get("kit")
	if len(img.Skills) != 0 {
		t.Fatalf("invented skills: %v", img.Skills)
	}
}

// A directory of loose files is not a skill, and saying so at load time beats
// publishing something the agent cannot read.
func TestSkillDirectoryWithoutASkillFileIsRejected(t *testing.T) {
	_, err := skillCatalog(map[string]string{"images/kit/skills/mount-bucket/notes.txt": "stray\n"})
	if err == nil {
		t.Fatal("accepted a skill directory with no SKILL.md")
	}
}

// Skill names become directories in a project workspace.
func TestSkillNameMustBeOnePathComponent(t *testing.T) {
	_, err := skillCatalog(map[string]string{"images/kit/skills/Mount Bucket/SKILL.md": "# no\n"})
	if err == nil {
		t.Fatal("accepted a skill name that is not a safe path component")
	}
}
