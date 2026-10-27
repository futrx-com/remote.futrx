package provisioning

import (
	"bytes"
	"testing"
)

func TestDefaultSkillsIncludesRemoteApplicationPackage(t *testing.T) {
	skills := DefaultSkills()
	var skill *DefaultSkill
	for index := range skills {
		if skills[index].Command == "remote-application" {
			skill = &skills[index]
			break
		}
	}
	if skill == nil {
		t.Fatalf("remote-application missing from default skills: %#v", skills)
	}

	wantPaths := []string{
		"SKILL.md",
		"agents/openai.yaml",
		"references/capability-selection.md",
	}
	if len(skill.Assets) != len(wantPaths) {
		t.Fatalf("default skill assets = %#v, want %q", skill.Assets, wantPaths)
	}
	for index, want := range wantPaths {
		if got := skill.Assets[index].Path; got != want {
			t.Fatalf("default skill asset %d = %q, want %q", index, got, want)
		}
	}
	if !bytes.Contains(skill.Assets[0].Content, []byte("name: remote-application")) {
		t.Fatal("remote application SKILL.md is missing its metadata")
	}
}

func TestDefaultSkillsReturnsIsolatedContent(t *testing.T) {
	first := DefaultSkills()
	if len(first) == 0 || len(first[0].Assets) == 0 || len(first[0].Assets[0].Content) == 0 {
		t.Fatal("default skill fixture is empty")
	}
	wantCommand := first[0].Command
	wantFirstByte := first[0].Assets[0].Content[0]
	first[0].Command = "changed"
	first[0].Assets[0].Content[0] = 'x'

	second := DefaultSkills()
	if second[0].Command != wantCommand {
		t.Fatalf("mutated command escaped copy: %q", second[0].Command)
	}
	if second[0].Assets[0].Content[0] != wantFirstByte {
		t.Fatal("mutated asset content escaped copy")
	}
}
