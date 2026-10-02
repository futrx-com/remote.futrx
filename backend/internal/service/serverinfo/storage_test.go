package serverinfo

import (
	"context"
	"testing"
	"time"
)

type storageCollector struct{ mounts []StorageMount }

func (c storageCollector) Collect(context.Context, time.Time) Snapshot {
	return Snapshot{Storage: StorageInfo{Mounts: append([]StorageMount(nil), c.mounts...)}}
}
func TestStorageWarningsCoverBytesInodesAndRecovery(t *testing.T) {
	inode := 91.0
	collector := storageCollector{mounts: []StorageMount{{MountPath: "/", UsagePercent: 80}, {MountPath: "/projects", UsagePercent: 30, InodePercent: &inode}}}
	svc := New(collector, "test", "/data", "/projects")
	got := svc.Collect(context.Background())
	if !got.Storage.Mounts[0].Warning || !got.Storage.Mounts[1].Warning {
		t.Fatal(got.Storage)
	}
	collector.mounts[0].UsagePercent = 70
	inode = 40
	got = svc.Collect(context.Background())
	if got.Storage.Mounts[0].Warning || got.Storage.Mounts[1].Warning {
		t.Fatal("warning did not recover")
	}
	svc.WithStorageWarningThreshold(60)
	if !svc.Collect(context.Background()).Storage.Mounts[0].Warning {
		t.Fatal("custom threshold ignored")
	}
}
