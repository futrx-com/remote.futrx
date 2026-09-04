package resources

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	serviceresources "github.com/futrx-com/remote.futrx.com/internal/service/resources"
)

// quotaDrivers are the LXD storage drivers that can enforce a root-disk
// `size`. `dir` cannot: LXD has no way to bound a plain directory, so a quota
// set against it is silently useless or rejected at start. `lxd init --auto`
// picks `dir` on a box with no spare block device and a zfs loop file
// otherwise, so both outcomes are live on real installs.
var quotaDrivers = map[string]bool{
	"btrfs": true,
	"ceph":  true,
	"lvm":   true,
	"zfs":   true,
}

const poolCacheTTL = 5 * time.Minute

// poolCache memoizes the storage-pool probe. The driver of a pool never
// changes without recreating it, and the probe costs two `lxc` round trips.
type poolCache struct {
	mu       sync.Mutex
	value    serviceresources.PoolCapability
	at       time.Time
	resolved bool
}

// PoolCapability reports whether the pool backing project containers can
// enforce disk quotas. Resolution walks the `default` profile's root device to
// find the pool name, then reads that pool's driver.
func (m *Manager) PoolCapability(ctx context.Context) (serviceresources.PoolCapability, error) {
	m.pool.mu.Lock()
	defer m.pool.mu.Unlock()
	if m.pool.resolved && time.Since(m.pool.at) < poolCacheTTL {
		return m.pool.value, nil
	}
	if !m.runner.Available() {
		return serviceresources.PoolCapability{}, command.ErrUnavailable
	}

	name := m.rootPoolName(ctx)
	if name == "" {
		return serviceresources.PoolCapability{
			Detail: "no root disk device found on the default profile - disk quotas cannot be applied",
		}, nil
	}
	shown, err := command.RunWithTimeout(ctx, m.runner, queryTimeout, "storage", "show", name)
	if err != nil {
		return serviceresources.PoolCapability{Pool: name}, fmt.Errorf("storage show %s: %w; output: %s", name, err, shown)
	}
	capability := describePool(name, ParseStorageDriver(shown))
	m.pool.value, m.pool.at, m.pool.resolved = capability, time.Now(), true
	return capability, nil
}

// quotaSupported is the internal, error-swallowing form used on the launch
// path: an unreachable LXD must not turn into a failed container start.
func (m *Manager) quotaSupported(ctx context.Context) bool {
	capability, err := m.PoolCapability(ctx)
	return err == nil && capability.Supported
}

func (m *Manager) rootPoolName(ctx context.Context) string {
	out, err := command.RunWithTimeout(ctx, m.runner, queryTimeout, "profile", "device", "get", "default", "root", "pool")
	if err == nil {
		if name := strings.TrimSpace(out); name != "" {
			return name
		}
	}
	return ""
}

func describePool(name, driver string) serviceresources.PoolCapability {
	capability := serviceresources.PoolCapability{Pool: name, Driver: driver}
	switch {
	case driver == "":
		capability.Detail = "storage driver could not be determined - disk quotas are reported as unsupported"
	case quotaDrivers[driver]:
		capability.Supported = true
	default:
		capability.Detail = fmt.Sprintf(
			"the %q storage driver cannot enforce a root disk quota - LXD needs btrfs, zfs, lvm, or ceph",
			driver,
		)
	}
	return capability
}

// ParseStorageDriver extracts the driver from `lxc storage show <pool>` YAML.
// Only the top-level `driver:` key counts; nested config keys are indented and
// must not be mistaken for it.
func ParseStorageDriver(output string) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimRight(line, "\r")
		if trimmed != strings.TrimLeft(trimmed, " \t-") {
			continue
		}
		rest, ok := strings.CutPrefix(trimmed, "driver:")
		if !ok {
			continue
		}
		return strings.ToLower(strings.Trim(strings.TrimSpace(rest), `"'`))
	}
	return ""
}
