// Package resources owns the fleet-wide container resource policy: the
// operator-editable defaults behind the managed LXD profile, the host reserve
// that keeps the control plane alive, the per-project override ceiling, and
// the aggregate admission guard that stops a host reboot from autostarting
// more workspaces than the box can hold.
//
// Until this package existed the fleet envelope was compiled into
// `integration/containers/resources`, so changing it meant editing source and
// redeploying. The policy now lives in `DATA_DIR/resources.json`, is derived
// from real host capacity on first run, and converges the managed profile
// whenever it changes.
package resources

import "errors"

var (
	// ErrInvalidSettings reports a malformed or self-contradictory policy.
	ErrInvalidSettings = errors.New("invalid resource settings")
	// ErrOverrideTooLarge reports a per-project override above the operator's
	// ceiling or above what the host can physically back.
	ErrOverrideTooLarge = errors.New("resource override exceeds the allowed maximum")
	// ErrHostMemoryExhausted reports that starting one more container would
	// commit more memory than the host has outside its reserve.
	ErrHostMemoryExhausted = errors.New("host memory budget exhausted")
	// ErrTooManyRunning reports that the operator's running-container cap is
	// already met.
	ErrTooManyRunning = errors.New("running container limit reached")
	// ErrUnavailable reports that the container runtime could not be reached,
	// so admission and convergence cannot be evaluated.
	ErrUnavailable = errors.New("container runtime unavailable")
)

// Limits is one resource envelope. Sizes use LXD's own byte-suffix grammar
// ("2GiB", "768MiB"); CPU is a core count that may be fractional in a reserve
// but is rounded to whole cores when written to LXD. A zero/empty field means
// "unset" and, for a project override, "inherit the fleet default".
type Limits struct {
	Memory    string  `json:"memory,omitempty"`
	CPU       float64 `json:"cpu,omitempty"`
	Processes int     `json:"processes,omitempty"`
	Disk      string  `json:"disk,omitempty"`
}

// Reserve is the slice of the host held back from workspaces so the backend,
// LXD, sshd, and Caddy keep running when every container is at its ceiling.
type Reserve struct {
	Memory string  `json:"memory,omitempty"`
	CPU    float64 `json:"cpu,omitempty"`
}

// Settings is the persisted policy document (`DATA_DIR/resources.json`).
type Settings struct {
	Defaults             Limits  `json:"defaults"`
	HostReserve          Reserve `json:"hostReserve"`
	MaxProjectOverride   Limits  `json:"maxProjectOverride"`
	MaxRunningContainers int     `json:"maxRunningContainers"`
	Derived              bool    `json:"derived,omitempty"`
	UpdatedAt            int64   `json:"updatedAt,omitempty"`
	UpdatedBy            string  `json:"updatedBy,omitempty"`
}

// HostFacts is the subset of host capacity the derivation and the aggregate
// guard need. Supplied by the same collector that backs `/api/server/info`.
type HostFacts struct {
	MemoryBytes uint64
	CPUs        int
	DiskBytes   uint64
}

// PoolCapability reports whether the LXD storage pool backing project
// containers can enforce a root-disk quota at all. `dir` pools cannot: LXD
// requires btrfs, zfs, lvm, or ceph. Detecting this lets the platform report
// "unsupported on this pool" instead of failing every launch.
type PoolCapability struct {
	Pool      string `json:"pool,omitempty"`
	Driver    string `json:"driver,omitempty"`
	Supported bool   `json:"supported"`
	Detail    string `json:"detail,omitempty"`
}

// Instance is one container as the aggregate guard sees it.
type Instance struct {
	Name    string
	Running bool
	Memory  string
}

// HostCapacity is the host-side arithmetic the UI shows next to the form.
type HostCapacity struct {
	MemoryBytes        uint64 `json:"memoryBytes"`
	CPUs               int    `json:"cpus"`
	DiskBytes          uint64 `json:"diskBytes"`
	ReserveMemoryBytes uint64 `json:"reserveMemoryBytes"`
	BudgetMemoryBytes  uint64 `json:"budgetMemoryBytes"`
	CommittedBytes     uint64 `json:"committedMemoryBytes"`
	RunningContainers  int    `json:"runningContainers"`
}

// View is the complete admin-facing state of the policy.
type View struct {
	Settings  Settings       `json:"settings"`
	Host      HostCapacity   `json:"host"`
	DiskQuota PoolCapability `json:"diskQuota"`
	Available bool           `json:"runtimeAvailable"`
}
