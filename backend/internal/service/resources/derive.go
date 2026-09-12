package resources

import "math"

// Derivation bounds. The memory floor keeps a workspace usable (Node plus an
// agent CLI); the ceiling is the envelope that survived the 2026-07 host
// takedowns and stays the most a fleet default ever grants unattended.
const (
	minDefaultMemory = 1 * gib
	maxDefaultMemory = 4 * gib
	memoryRounding   = 512 * mib

	minDefaultDisk = 5 * gib
	maxDefaultDisk = 20 * gib
	diskRounding   = 1 * gib

	defaultProcesses = 2000

	// DefaultReserveMemory and DefaultReserveCPU hold back enough for the Go
	// backend, LXD, Caddy, and sshd on the smallest supported box.
	DefaultReserveMemory = "768MiB"
	DefaultReserveCPU    = 0.5
)

// DefaultReserve is the host reserve applied when the operator has not set
// one.
func DefaultReserve() Reserve {
	return Reserve{Memory: DefaultReserveMemory, CPU: DefaultReserveCPU}
}

// DeriveDefaults computes the fleet default envelope from real host capacity.
// It runs once, on the first start that finds no `resources.json`, so a 1
// vCPU / 4 GiB box does not hand a single container the whole machine the way
// the old compiled-in 4 GiB / 6 CPU default did.
//
// Memory takes what remains after the host reserve, clamped into
// [1 GiB, 4 GiB] and floored onto a 512 MiB boundary so the value reads
// cleanly. CPU takes the whole cores left after the reserve, never below one.
// Disk takes a quarter of the host filesystem, clamped into [5 GiB, 20 GiB].
// A fact the collector could not read (zero) falls back to the floor.
func DeriveDefaults(facts HostFacts, reserve Reserve) Limits {
	return Limits{
		Memory:    FormatSize(deriveMemory(facts.MemoryBytes, reserve.Memory)),
		CPU:       deriveCPU(facts.CPUs, reserve.CPU),
		Processes: defaultProcesses,
		Disk:      FormatSize(deriveDisk(facts.DiskBytes)),
	}
}

func deriveMemory(hostBytes uint64, reserve string) uint64 {
	if hostBytes == 0 {
		// The collector could not read host memory. Fall back to the floor
		// rather than inventing capacity the box may not have.
		return minDefaultMemory
	}
	reserveBytes, err := ParseSize(reserve)
	if err != nil {
		reserveBytes = 0
	}
	available := uint64(0)
	if hostBytes > reserveBytes {
		available = hostBytes - reserveBytes
	}
	if available > maxDefaultMemory {
		available = maxDefaultMemory
	}
	if available >= minDefaultMemory {
		return roundDownTo(available, memoryRounding)
	}
	// A host too small for the normal floor still needs a workable default:
	// take what the reserve leaves, on a 256 MiB boundary, never below the
	// smallest envelope the policy accepts at all.
	small := roundDownTo(available, 256*mib)
	if small < minSettableMemory {
		return minSettableMemory
	}
	return small
}

func deriveCPU(hostCPUs int, reserve float64) float64 {
	if hostCPUs < 1 {
		return 1
	}
	usable := math.Floor(float64(hostCPUs) - reserve)
	if usable < 1 {
		return 1
	}
	return usable
}

func deriveDisk(hostBytes uint64) uint64 {
	quarter := roundDownTo(hostBytes/4, diskRounding)
	if quarter > maxDefaultDisk {
		return maxDefaultDisk
	}
	if quarter < minDefaultDisk {
		return minDefaultDisk
	}
	return quarter
}

// DeriveSettings builds the complete first-run policy document.
func DeriveSettings(facts HostFacts) Settings {
	reserve := DefaultReserve()
	defaults := DeriveDefaults(facts, reserve)
	return Settings{
		Defaults:           defaults,
		HostReserve:        reserve,
		MaxProjectOverride: deriveOverrideCeiling(facts, reserve, defaults),
		Derived:            true,
	}
}

// deriveOverrideCeiling caps a per-project override at what the host can back
// on its own: everything outside the reserve, every core, and the whole
// filesystem. The aggregate guard still refuses a start that would oversubscribe
// the host once several projects claim the ceiling at the same time.
func deriveOverrideCeiling(facts HostFacts, reserve Reserve, defaults Limits) Limits {
	reserveBytes, _ := ParseSize(reserve.Memory)
	ceiling := uint64(0)
	if facts.MemoryBytes > reserveBytes {
		ceiling = roundDownTo(facts.MemoryBytes-reserveBytes, memoryRounding)
	}
	defaultBytes, _ := ParseSize(defaults.Memory)
	if ceiling < defaultBytes {
		ceiling = defaultBytes
	}
	cpus := facts.CPUs
	if cpus < int(defaults.CPU) {
		cpus = int(defaults.CPU)
	}
	if cpus < 1 {
		cpus = 1
	}
	disk := roundDownTo(facts.DiskBytes, diskRounding)
	defaultDisk, _ := ParseSize(defaults.Disk)
	if disk < defaultDisk {
		disk = defaultDisk
	}
	return Limits{
		Memory:    FormatSize(ceiling),
		CPU:       float64(cpus),
		Processes: defaultProcesses * 4,
		Disk:      FormatSize(disk),
	}
}
