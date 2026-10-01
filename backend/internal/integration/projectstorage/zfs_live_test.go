package projectstorage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// This opt-in test writes bounded disposable data, never creates/destroys pools,
// and retains its datasets for inspection. Run only on a disposable QA guest.
func TestZFSQuotaOnDisposableHost(t *testing.T) {
	parent := os.Getenv("REMOTE_ZFS_QA_DATASET")
	if parent == "" {
		t.Skip("set REMOTE_ZFS_QA_DATASET and REMOTE_ZFS_QA_ROOT on a disposable QA host")
	}
	root := os.Getenv("REMOTE_ZFS_QA_ROOT")
	if !strings.HasSuffix(parent, "/remote-qa") || !filepath.IsAbs(root) || root == "/" || root == "/workspace" || root == "/var/lib/remote/projects" {
		t.Fatal("use a dedicated remote-qa dataset and disposable mountpoint")
	}
	slug := fmt.Sprintf("quota-%d", time.Now().UnixNano())
	p := project.Meta{ID: "aabbccdd", Cwd: filepath.Join(root, slug, "workspace")}
	m := New(root, parent, "32MiB", true)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := m.Ensure(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.Cwd, 0755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(filepath.Dir(p.Cwd), "agent-home", "codex")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(home, "qa-marker")
	if err := os.WriteFile(marker, []byte("persistent-state"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(p.Cwd, "quota-probe"))
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 1024*1024)
	if _, err = rand.Read(data); err != nil {
		t.Fatal(err)
	}
	var writeErr error
	for n := 0; n < 64; n++ {
		if _, writeErr = file.Write(data); writeErr != nil {
			break
		}
		if writeErr = file.Sync(); writeErr != nil {
			break
		}
	}
	file.Close()
	if !errors.Is(writeErr, syscall.EDQUOT) && !errors.Is(writeErr, syscall.ENOSPC) {
		t.Fatalf("expected quota exhaustion, got %v", writeErr)
	}
	if err := os.Remove(filepath.Join(p.Cwd, "quota-probe")); err != nil {
		t.Fatal(err)
	}
	// Recreating the manager mirrors a backend restart; the dataset remains.
	again := New(root, parent, "64MiB", true)
	if err := again.Ensure(ctx, p); err != nil {
		t.Fatal(err)
	}
	info := again.Inspect(ctx, p.Cwd)
	if !info.Enforced || info.LimitBytes == nil || *info.LimitBytes != 32<<20 {
		t.Fatalf("existing quota changed after recreation: %#v", info)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "persistent-state" {
		t.Fatalf("provider state lost: %v", err)
	}
	p.ID = "eeff0011"
	if err := again.Ensure(ctx, p); err == nil {
		t.Fatal("another project adopted retained provider state")
	}
	t.Logf("verified real quota exhaustion and retained state at %s", p.Cwd)
}
