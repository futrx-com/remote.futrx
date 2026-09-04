package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProgressWriterPublishesAtomicStructuredState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "progress.json")
	writer := newProgressWriter(path)
	writer.now = func() time.Time { return time.Unix(1234, 0) }
	if err := writer.write(2, 7, "astrology", "Recycling workspace 3 of 7"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got workspaceProgress
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := workspaceProgress{
		Phase: "workspace-migration", Message: "Recycling workspace 3 of 7",
		Completed: 2, Total: 7, CurrentItem: "astrology", UpdatedAt: 1234,
	}
	if got != want {
		t.Fatalf("progress = %+v, want %+v", got, want)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("progress permissions = %v, err = %v", info.Mode().Perm(), err)
	}
}
