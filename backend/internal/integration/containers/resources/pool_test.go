package resources

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// Fixtures captured from `lxc storage show <pool>` on the two layouts
// `lxd init --auto` produces: a plain directory pool on a box with no spare
// block device, and a zfs loop file everywhere else.
const (
	dirPoolShow = `config:
  source: /var/snap/lxd/common/lxd/storage-pools/default
description: ""
name: default
driver: dir
used_by:
- /1.0/instances/alpha
- /1.0/profiles/default
status: Created
locations:
- none
`

	zfsPoolShow = `config:
  size: 30GiB
  source: default
  zfs.pool_name: default
description: ""
name: default
driver: zfs
used_by:
- /1.0/profiles/default
status: Created
locations:
- none
`

	btrfsPoolShow = "name: tank\ndriver: btrfs\n"

	// A pool whose own config mentions a driver-shaped key must not be
	// mistaken for the top-level driver.
	nestedDriverShow = `config:
  lvm.thinpool_name: LXDThinPool
  driver: decoy
description: ""
name: vg0
driver: lvm
status: Created
`
)

func TestParseStorageDriver(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "dir pool", output: dirPoolShow, want: "dir"},
		{name: "zfs pool", output: zfsPoolShow, want: "zfs"},
		{name: "btrfs pool", output: btrfsPoolShow, want: "btrfs"},
		{name: "nested key is ignored", output: nestedDriverShow, want: "lvm"},
		{name: "quoted value", output: "driver: \"ceph\"\n", want: "ceph"},
		{name: "carriage returns", output: "name: p\r\ndriver: zfs\r\n", want: "zfs"},
		{name: "no driver key", output: "name: p\nstatus: Created\n", want: ""},
		{name: "empty output", output: "", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ParseStorageDriver(test.output); got != test.want {
				t.Fatalf("ParseStorageDriver = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDescribePoolQuotaSupport(t *testing.T) {
	tests := []struct {
		driver        string
		wantSupported bool
		wantDetail    bool
	}{
		{driver: "zfs", wantSupported: true},
		{driver: "btrfs", wantSupported: true},
		{driver: "lvm", wantSupported: true},
		{driver: "ceph", wantSupported: true},
		{driver: "dir", wantSupported: false, wantDetail: true},
		{driver: "", wantSupported: false, wantDetail: true},
	}

	for _, test := range tests {
		t.Run("driver="+test.driver, func(t *testing.T) {
			got := describePool("default", test.driver)
			if got.Supported != test.wantSupported {
				t.Fatalf("Supported = %t, want %t", got.Supported, test.wantSupported)
			}
			if (got.Detail != "") != test.wantDetail {
				t.Fatalf("Detail = %q, want present=%t", got.Detail, test.wantDetail)
			}
			if got.Pool != "default" {
				t.Fatalf("Pool = %q, want default", got.Pool)
			}
		})
	}
}

func TestPoolCapabilityReadsDefaultProfileRootPool(t *testing.T) {
	runner := &fakeRunner{responses: map[string]fakeResponse{
		"profile device get default root pool": {out: "default\n"},
		"storage show default":                 {out: dirPoolShow},
	}}

	got, err := NewManager(runner).PoolCapability(context.Background())
	if err != nil {
		t.Fatalf("PoolCapability: %v", err)
	}
	if got.Driver != "dir" || got.Supported {
		t.Fatalf("capability = %+v, want unsupported dir pool", got)
	}
}

func TestPoolCapabilityWithoutRootDeviceReportsUnsupported(t *testing.T) {
	runner := &fakeRunner{responses: map[string]fakeResponse{
		"profile device get default root pool": {err: errors.New("device not found")},
	}}

	got, err := NewManager(runner).PoolCapability(context.Background())
	if err != nil {
		t.Fatalf("PoolCapability: %v", err)
	}
	if got.Supported || got.Detail == "" {
		t.Fatalf("capability = %+v, want unsupported with a reason", got)
	}
}

func TestSetLimitsSkipsDiskQuotaOnUnsupportedPool(t *testing.T) {
	runner := &fakeRunner{responses: map[string]fakeResponse{
		"profile device get default root pool": {out: "default\n"},
		"storage show default":                 {out: dirPoolShow},
	}}

	if err := NewManager(runner).SetLimits(context.Background(), "c1", "2", "2GiB", "20GiB"); err != nil {
		t.Fatalf("SetLimits: %v", err)
	}

	want := []string{
		"config set c1 limits.cpu 2",
		"config set c1 limits.memory 2GiB",
		"profile device get default root pool",
		"storage show default",
	}
	if !slices.Equal(runner.calls, want) {
		t.Fatalf("calls:\n got: %q\nwant: %q", runner.calls, want)
	}
}
