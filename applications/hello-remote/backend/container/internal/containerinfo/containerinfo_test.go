package containerinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReaders(t *testing.T) {
	if got, err := readOSName(fixture(t, "PRETTY_NAME=\"Ubuntu 24.04 LTS\"\n")); err != nil || got != "Ubuntu 24.04 LTS" {
		t.Fatalf("OS = %q, %v", got, err)
	}
	if got, err := readMemoryTotal(fixture(t, "MemTotal: 2048 kB\n")); err != nil || got != 2*1024*1024 {
		t.Fatalf("memory = %d, %v", got, err)
	}
	if got, err := readUptime(fixture(t, "1234.56 1000.00\n")); err != nil || got != 1234 {
		t.Fatalf("uptime = %d, %v", got, err)
	}
}
