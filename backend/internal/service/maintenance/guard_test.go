package maintenance

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGuardOnlyBlocksForLiveFreshOwner(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	dir := t.TempDir()
	path := filepath.Join(dir, "self-update", "maintenance.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	guard := New(dir)
	guard.now = func() time.Time { return now }
	guard.alive = func(pid int) bool { return pid == 42 }

	write := func(contents string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write(`{"pid":42,"startedAt":1799999940}`)
	if !guard.Blocked() {
		t.Fatal("fresh marker owned by a live process did not block")
	}

	write(`{"pid":99,"startedAt":1799999940}`)
	if guard.Blocked() {
		t.Fatal("marker owned by a dead process blocked")
	}

	write(`{"pid":42,"startedAt":1799990000}`)
	if guard.Blocked() {
		t.Fatal("stale marker blocked")
	}

	write(`not json`)
	if guard.Blocked() {
		t.Fatal("invalid marker blocked")
	}
}
