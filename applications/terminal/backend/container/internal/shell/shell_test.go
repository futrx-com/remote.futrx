//go:build linux

package shell

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestWorkingDirectoryStaysInsideTheWorkspace(t *testing.T) {
	workspace := realPath(t, t.TempDir())
	outside := realPath(t, t.TempDir())
	if err := os.Mkdir(filepath.Join(workspace, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		t.Fatal(err)
	}
	for requested, want := range map[string]string{
		filepath.Join(workspace, "api"):       filepath.Join(workspace, "api"),
		workspace:                             workspace,
		"":                                    workspace,
		"api":                                 workspace,
		outside:                               workspace,
		filepath.Join(workspace, "..", "etc"): workspace,
		filepath.Join(workspace, "api", "..", "..", filepath.Base(outside)): workspace,
		filepath.Join(workspace, "escape"):                                  workspace,
		filepath.Join(workspace, "missing"):                                 workspace,
		filepath.Join(workspace, "file"):                                    workspace,
		workspace + "-sibling":                                              workspace,
	} {
		if got := WorkingDirectory(workspace, requested); got != want {
			t.Errorf("WorkingDirectory(%q) = %q, want %q", requested, got, want)
		}
	}
}

func TestShellEnvironmentKeepsOnlyTheContainersPath(t *testing.T) {
	env := Environment(func(name string) string {
		if name == "PATH" {
			return "/opt/bin:/usr/bin"
		}
		return "leaked"
	})
	if !slices.Contains(env, "PATH=/opt/bin:/usr/bin") || !slices.Contains(env, "TERM=xterm-256color") ||
		slices.ContainsFunc(env, func(entry string) bool { return strings.Contains(entry, "leaked") }) {
		t.Fatalf("environment = %q", env)
	}
	if fallback := Environment(func(string) string { return "" }); !slices.Contains(fallback,
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin") {
		t.Fatalf("fallback environment = %q", fallback)
	}
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
