package resources

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/lxc"
)

// Opt-in acceptance against the minimal QA image in a disposable LXD guest.
func TestRootQuotaOnDisposableLXD(t *testing.T) {
	instance := os.Getenv("REMOTE_LXD_QA_INSTANCE")
	if instance == "" {
		t.Skip("set REMOTE_LXD_QA_INSTANCE on a disposable QA host")
	}
	if instance != "remote-migrate-qa" {
		t.Fatal("only the named disposable QA instance is accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	runner := lxc.New()
	manager := NewManager(runner).WithDefaultDisk("64MiB")
	if err := manager.Ensure(ctx, instance); err != nil {
		t.Fatal(err)
	}
	cap, size, err := manager.DiskCapability(ctx, instance)
	if err != nil || !cap.Supported || cap.Driver != "zfs" || size != "64MiB" {
		t.Fatalf("effective quota: %#v size=%s err=%v", cap, size, err)
	}
	out, err := runner.Run(ctx, "exec", instance, "--", "/bin/qa", "fill", "/quota-probe", "128")
	if err == nil || (!strings.Contains(out, "quota exceeded") && !strings.Contains(out, "no space left")) {
		t.Fatalf("quota did not reject a real write: %v %s", err, out)
	}
	if out, err = runner.Run(ctx, "file", "delete", instance+"/quota-probe"); err != nil {
		t.Fatalf("remove disposable probe: %v %s", err, out)
	}
	t.Log("verified root quota from the actual instance pool and a rejected write")
}
