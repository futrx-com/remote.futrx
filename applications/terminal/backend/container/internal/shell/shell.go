//go:build linux

// Package shell describes the login shell a terminal session runs.
package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Command returns how to start a shell for a session that asked to begin in
// cwd. The shell always starts somewhere inside workspace.
func Command(workspace string, getenv func(string) string) func(cwd string) *exec.Cmd {
	return func(cwd string) *exec.Cmd {
		cmd := exec.Command("/bin/bash", "-l")
		cmd.Env = Environment(getenv)
		cmd.Dir = WorkingDirectory(workspace, cwd)
		return cmd
	}
}

// Environment is the shell's whole environment. Only PATH is taken from the
// service, so nothing else the service was started with leaks into shells.
func Environment(getenv func(string) string) []string {
	path := getenv("PATH")
	if path == "" {
		path = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	return []string{
		"TERM=xterm-256color",
		"HOME=/root",
		"USER=root",
		"LOGNAME=root",
		"SHELL=/bin/bash",
		"LANG=C.UTF-8",
		"PATH=" + path,
	}
}

// WorkingDirectory returns the directory a new shell starts in: the requested
// one when it is an existing directory inside the workspace, the workspace
// itself otherwise. Symlinks are resolved first, so a link pointing out of the
// workspace does not qualify.
func WorkingDirectory(workspace, requested string) string {
	if !filepath.IsAbs(requested) {
		return workspace
	}
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return workspace
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(requested))
	if err != nil {
		return workspace
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return workspace
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return workspace
	}
	return resolved
}
