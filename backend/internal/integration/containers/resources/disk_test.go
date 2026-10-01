package resources

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDefaultQuotaFollowsActualInstancePool(t *testing.T) {
	for _, driver := range []string{"dir", "zfs", "btrfs"} {
		runner := &fakeRunner{responses: map[string]fakeResponse{
			"query /1.0/instances/c1": {out: `{"expanded_devices":{"root":{"pool":"migrated"}}}`},
			"storage show migrated":   {out: "driver: " + driver},
		}}
		if err := NewManager(runner).ensureDefaultDisk(context.Background(), "c1"); err != nil {
			t.Fatal(err)
		}
		calls := runner.called("config device override")
		want := 0
		if driver != "dir" {
			want = 1
		}
		if len(calls) != want {
			t.Fatalf("driver %s calls %v", driver, runner.calls)
		}
		if want == 1 && !strings.Contains(calls[0], "size=20GiB") {
			t.Fatal(calls)
		}
	}
}
func TestQuotaFailureDoesNotPretendToApply(t *testing.T) {
	runner := &fakeRunner{responses: map[string]fakeResponse{"query /1.0/instances/c1": {err: errors.New("LXD unavailable")}}}
	if err := NewManager(runner).ensureDefaultDisk(context.Background(), "c1"); err == nil {
		t.Fatal("unknown capability accepted")
	}
	if len(runner.called("config device")) != 0 {
		t.Fatal(runner.calls)
	}
}
