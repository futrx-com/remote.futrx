package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInstructionsStorePersistsAndRejectsInvalidReplacement(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "state", "instructions.json")
	store := NewAgentInstructionsStore(filename, instructionTestProfiles())
	if _, err := store.Read(); err != nil {
		t.Fatal(err)
	}
	valid := []byte(`{"global":"policy","providers":{"codex":"extra"}}`)
	if err := store.Write(valid); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{[]byte(`null`), []byte(`{"secret":true}`), []byte(`{"providers":{"unknown":"text"}}`), []byte(`{} {}`), bytes.Repeat([]byte("x"), maxAgentInstructionsBytes+1)} {
		if err := store.Write(invalid); err == nil {
			t.Fatal("accepted invalid instructions")
		}
		data, err := NewAgentInstructionsStore(filename, instructionTestProfiles()).Read()
		if err != nil || !bytes.Equal(data, valid) {
			t.Fatalf("lost valid settings: %s %v", data, err)
		}
	}
	stat, err := os.Stat(filename)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatalf("unsafe settings permissions: %v %v", stat, err)
	}
	if len(store.Targets()) != 2 || store.Targets()["claude"] != "/root/.claude/CLAUDE.md" {
		t.Fatal("wrong provider targets")
	}
}
