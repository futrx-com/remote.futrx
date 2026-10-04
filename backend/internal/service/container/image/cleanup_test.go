package image

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupRemovesDownloadsAndPreservesRuntimeAssets(t *testing.T) {
	root := t.TempDir()
	removed := []string{
		"var/lib/apt/lists/partial/package", "var/lib/apt/lists/.hidden",
		"root/.npm/_npx/package", "root/.npm/_logs/install.log",
		"root/.cache/pip/wheel", "tmp/pw-vendor/chrome.zip", "tmp/code-server.deb",
		"usr/local/bin/agy.1.old",
	}
	retained := []string{
		"root/.cache/ms-playwright/chromium/chrome", "root/.claude/settings.json",
		"root/.codex/config.toml", "usr/local/bin/agy", "usr/local/bin/other.old",
		"usr/local/lib/node_modules/provider/index.js", "workspace/setup.sh",
	}
	for _, name := range append(append([]string{}, removed...), retained...) {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	// Mock package-manager commands; execute the actual file cleanup against a
	// disposable fixture, never against the machine running the tests.
	for _, command := range []string{"apt-get", "npm"} {
		if err := os.WriteFile(filepath.Join(bin, command), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CLEANUP_COMMAND_LOG\"\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	var replacements []string
	for _, prefix := range []string{"/var/lib/apt/lists", "/root/", "/tmp/", "/usr/local/bin"} {
		replacements = append(replacements, prefix, root+prefix)
	}
	script := strings.NewReplacer(replacements...).Replace(cleanupScript)
	logPath := filepath.Join(root, "commands")
	for iteration := 0; iteration < 2; iteration++ {
		cmd := exec.Command("sh", "-c", script)
		cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "CLEANUP_COMMAND_LOG="+logPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cleanup iteration %d: %v: %s", iteration, err, out)
		}
	}
	for _, name := range removed {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Errorf("disposable file %s remains (error %v)", name, err)
		}
	}
	for _, name := range retained {
		if data, err := os.ReadFile(filepath.Join(root, name)); err != nil || string(data) != "fixture" {
			t.Errorf("runtime asset %s changed: %v", name, err)
		}
	}
	commands, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-o DPkg::Lock::Timeout=300 clean", "cache clean --force"} {
		if strings.Count(string(commands), want) != 2 {
			t.Errorf("missing package manager cleanup %q: %s", want, commands)
		}
	}
}
