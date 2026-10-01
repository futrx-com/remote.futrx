package projectstorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/project"
)

type fakeZFS struct {
	root                string
	exists              bool
	quota               uint64
	owner               string
	childMount          string
	mounted, visible    bool
	failCreate, failSet bool
	calls               []string
}

func (f *fakeZFS) run(_ context.Context, command string, args ...string) (string, error) {
	f.calls = append(f.calls, command+" "+strings.Join(args, " "))
	if command == "findmnt" {
		if !f.visible {
			return "", errors.New("not mounted")
		}
		if args[len(args)-1] == f.root {
			return "tank/projects\n", nil
		}
		return "tank/projects/demo\n", nil
	}
	if command != "zfs" {
		return "", errors.New("unexpected command")
	}
	switch args[0] {
	case "list":
		result := fmt.Sprintf("tank/projects\t%s\tyes\t0\n", f.root)
		if f.exists {
			mounted := "no"
			if f.mounted {
				mounted = "yes"
			}
			result += fmt.Sprintf("tank/projects/demo\t%s\t%s\t%d\n", f.childMount, mounted, f.quota)
		}
		return result, nil
	case "get":
		return f.owner, nil
	case "create":
		if f.failCreate {
			return "", errors.New("pool full")
		}
		f.exists = true
		f.mounted = true
		for _, a := range args {
			if strings.HasPrefix(a, "quota=") {
				f.quota, _ = strconv.ParseUint(strings.TrimPrefix(a, "quota="), 10, 64)
			}
			if strings.HasPrefix(a, "remote:project-id=") {
				f.owner = strings.TrimPrefix(a, "remote:project-id=")
			}
		}
		return "", os.MkdirAll(f.childMount, 0755)
	case "set":
		if f.failSet {
			return "", errors.New("quota rejected")
		}
		f.quota, _ = strconv.ParseUint(strings.TrimPrefix(args[1], "quota="), 10, 64)
		return "", nil
	}
	return "", errors.New("unexpected command")
}
func fixture(t *testing.T) (*Manager, *fakeZFS, project.Meta) {
	t.Helper()
	root := t.TempDir()
	f := &fakeZFS{root: root, childMount: filepath.Join(root, "demo"), mounted: true, visible: true, owner: "abcd"}
	m := New(root, "tank/projects", "20GiB", true)
	m.run = f.run
	return m, f, project.Meta{ID: "abcd", Cwd: filepath.Join(root, "demo", "workspace")}
}
func TestQuotaCoversParentOfWorkspaceAndHomesAndSurvivesRecreation(t *testing.T) {
	m, f, p := fixture(t)
	if e := m.Ensure(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"workspace/cache", "agent-home/claude", "agent-home/codex"} {
		if e := os.MkdirAll(filepath.Join(f.childMount, name), 0755); e != nil {
			t.Fatal(e)
		}
	}
	if e := m.Ensure(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, "zfs create") {
			count++
			if !strings.Contains(c, "mountpoint="+f.childMount) || !strings.Contains(c, "compression=lz4") {
				t.Fatal(c)
			}
		}
	}
	if count != 1 {
		t.Fatalf("created %d datasets", count)
	}
	q := m.Inspect(context.Background(), p.Cwd)
	if !q.Enforced || q.LimitBytes == nil || *q.LimitBytes != 20<<30 {
		t.Fatalf("quota %#v", q)
	}
}
func TestExistingOrdinaryDirectoryIsNeverCoveredByNewMount(t *testing.T) {
	m, f, p := fixture(t)
	if e := os.MkdirAll(p.Cwd, 0755); e != nil {
		t.Fatal(e)
	}
	marker := filepath.Join(p.Cwd, "valuable")
	os.WriteFile(marker, []byte("retained"), 0600)
	if e := m.Ensure(context.Background(), p); e == nil {
		t.Fatal("accepted migration without operator")
	}
	if f.exists {
		t.Fatal("created over existing directory")
	}
	if b, _ := os.ReadFile(marker); string(b) != "retained" {
		t.Fatal("lost data")
	}
}
func TestQuotaFailureAndMountMismatchFailClosed(t *testing.T) {
	for _, scenario := range []string{"create failure", "quota failure", "wrong mount", "unmounted", "hidden mount", "wrong owner", "invalid size"} {
		t.Run(scenario, func(t *testing.T) {
			m, f, p := fixture(t)
			switch scenario {
			case "create failure":
				f.failCreate = true
			case "quota failure":
				f.exists = true
				f.failSet = true
			case "wrong mount":
				f.exists = true
				f.childMount = "/somewhere/else"
			case "unmounted":
				f.exists = true
				f.mounted = false
			case "hidden mount":
				f.visible = false
			case "wrong owner":
				f.exists = true
				f.owner = "ffff"
			case "invalid size":
				m.size = "unlimited"
			}
			if e := m.Ensure(context.Background(), p); e == nil {
				t.Fatal("accepted unenforced persistent storage")
			}
		})
	}
}
func TestExistingFiniteQuotaIsPreserved(t *testing.T) {
	m, f, p := fixture(t)
	f.exists = true
	f.quota = 8 << 30
	if e := m.Ensure(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	if f.quota != 8<<30 {
		t.Fatal("overrode operator quota")
	}
}
func TestRequiredModeAndRedirectedPaths(t *testing.T) {
	m, f, p := fixture(t)
	m.dataset = ""
	if e := m.Ensure(context.Background(), p); e == nil {
		t.Fatal("required quota silently skipped")
	}
	if len(f.calls) != 0 {
		t.Fatal(f.calls)
	}
	m.required = false
	if e := m.Ensure(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	if m.Inspect(context.Background(), p.Cwd).Enforced {
		t.Fatal("claimed enforcement")
	}
	m.dataset = "tank/projects"
	os.Symlink(t.TempDir(), f.childMount)
	if e := m.Ensure(context.Background(), p); e == nil {
		t.Fatal("followed redirected project")
	}
	p.Cwd = filepath.Join(t.TempDir(), "workspace")
	if e := m.Ensure(context.Background(), p); e == nil {
		t.Fatal("accepted outside path")
	}
}
