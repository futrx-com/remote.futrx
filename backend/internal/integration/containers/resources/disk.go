package resources

// Driver capability detection follows PR #48 (mustafzidan74). This adapter
// resolves each instance's actual root pool, including migrated instances.
import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	"regexp"
	"strings"
)

const DefaultRootDiskQuota = "20GiB"

var diskSize = regexp.MustCompile(`^[1-9][0-9]*(MiB|GiB|TiB)$`)

type DiskCapability struct {
	Pool      string `json:"pool,omitempty"`
	Driver    string `json:"driver,omitempty"`
	Supported bool   `json:"supported"`
	Detail    string `json:"detail,omitempty"`
}

func (m *Manager) WithDefaultDisk(size string) *Manager {
	if size != "" {
		m.defaultDisk = size
	}
	return m
}
func (m *Manager) DiskCapability(ctx context.Context, name string) (DiskCapability, string, error) {
	out, err := command.RunWithTimeout(ctx, m.runner, queryTimeout, "query", "/1.0/instances/"+name)
	if err != nil {
		return DiskCapability{}, "", err
	}
	var instance struct {
		Devices map[string]map[string]string `json:"expanded_devices"`
	}
	if err := json.Unmarshal([]byte(out), &instance); err != nil {
		return DiskCapability{}, "", err
	}
	root := instance.Devices["root"]
	pool := root["pool"]
	if pool == "" {
		return DiskCapability{Detail: "root storage pool is unknown"}, root["size"], nil
	}
	out, err = command.RunWithTimeout(ctx, m.runner, queryTimeout, "storage", "show", pool)
	if err != nil {
		return DiskCapability{}, "", err
	}
	driver := ParseStorageDriver(out)
	cap := DiskCapability{Pool: pool, Driver: driver}
	switch driver {
	case "zfs", "btrfs", "lvm", "ceph":
		cap.Supported = true
	default:
		cap.Detail = "root quotas unsupported on storage driver " + driver
	}
	return cap, root["size"], nil
}
func ParseStorageDriver(yaml string) string {
	for _, line := range strings.Split(yaml, "\n") {
		if strings.HasPrefix(line, "driver:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "driver:")), "\"'")
		}
	}
	return ""
}
func (m *Manager) ensureDefaultDisk(ctx context.Context, name string) error {
	if !diskSize.MatchString(m.defaultDisk) {
		return fmt.Errorf("invalid default root disk quota")
	}
	cap, size, err := m.DiskCapability(ctx, name)
	if err != nil {
		return fmt.Errorf("inspect root quota support: %w", err)
	}
	if !cap.Supported || size != "" {
		return nil
	}
	return m.setDiskLimit(ctx, name, m.defaultDisk)
}
