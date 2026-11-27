package containerio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeLXC puts an `lxc` on PATH that records its arguments and stdin, then
// answers as the container program would.
func fakeLXC(t *testing.T, body string) (record string) {
	t.Helper()
	dir := t.TempDir()
	record = filepath.Join(dir, "record")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + record + ".args\ncat > " + record + ".stdin\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "lxc"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return record
}

func TestRunPassesBrowserInputOnStdinOnly(t *testing.T) {
	record := fakeLXC(t, `printf '{"ok":true}'`)
	output, err := Run(context.Background(), "project-demo", "list", Request{Path: "a b; rm -rf /"}, 1024)
	if err != nil || string(output) != `{"ok":true}` {
		t.Fatalf("Run = %q, %v", output, err)
	}
	args, _ := os.ReadFile(record + ".args")
	if string(args) != "exec\nproject-demo\n--\n/usr/local/bin/file-manager\nlist\n" {
		t.Fatalf("lxc arguments = %q", args)
	}
	stdin, _ := os.ReadFile(record + ".stdin")
	if string(stdin) != `{"path":"a b; rm -rf /"}` {
		t.Fatalf("stdin = %q", stdin)
	}
}

func TestRunMapsContainerExitStatuses(t *testing.T) {
	for status, want := range map[string]error{
		"3": ErrInvalidPath,
		"4": ErrNotFound,
		"5": ErrTooLarge,
	} {
		fakeLXC(t, "exit "+status)
		if _, err := Run(context.Background(), "project-demo", "archive", Request{}, 1024); !errors.Is(err, want) {
			t.Errorf("exit %s = %v, want %v", status, err, want)
		}
	}
	fakeLXC(t, "echo 'Error: Instance is not running' >&2; exit 1")
	if _, err := Run(context.Background(), "project-demo", "list", Request{}, 1024); err == nil ||
		!strings.Contains(err.Error(), "Instance is not running") {
		t.Fatalf("lxc failure = %v", err)
	}
}

func TestRunRefusesOutputPastTheLimit(t *testing.T) {
	fakeLXC(t, "head -c 4096 /dev/zero")
	if _, err := Run(context.Background(), "project-demo", "archive", Request{}, 1024); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized output = %v", err)
	}
}
