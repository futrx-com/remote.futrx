package resources

import (
	"fmt"
	"strings"
)

// Envelope bounds accepted from an operator. They exist to stop a typo
// ("2MiB", "9999 CPUs") from bricking every workspace on the box, not to
// express policy - policy is the operator's to set inside these bounds.
const (
	minSettableMemory = 256 * mib
	maxSettableCPU    = 256
	minSettableDisk   = 1 * gib
	maxSettableDisk   = 64 * tib
	maxSettableProcs  = 1 << 20
)

// Validate checks one complete policy document for internal consistency and
// against real host capacity.
func Validate(settings Settings, facts HostFacts) error {
	defaultMemory, err := checkMemory("defaults.memory", settings.Defaults.Memory, true)
	if err != nil {
		return err
	}
	if settings.Defaults.CPU < 1 || settings.Defaults.CPU > maxSettableCPU {
		return fmt.Errorf("%w: defaults.cpu must be between 1 and %d", ErrInvalidSettings, maxSettableCPU)
	}
	if settings.Defaults.Processes < 1 || settings.Defaults.Processes > maxSettableProcs {
		return fmt.Errorf("%w: defaults.processes must be between 1 and %d", ErrInvalidSettings, maxSettableProcs)
	}
	defaultDisk, err := checkDisk("defaults.disk", settings.Defaults.Disk)
	if err != nil {
		return err
	}

	reserve, err := checkMemory("hostReserve.memory", settings.HostReserve.Memory, false)
	if err != nil {
		return err
	}
	if settings.HostReserve.CPU < 0 || settings.HostReserve.CPU > maxSettableCPU {
		return fmt.Errorf("%w: hostReserve.cpu must be between 0 and %d", ErrInvalidSettings, maxSettableCPU)
	}

	maxMemory, err := checkMemory("maxProjectOverride.memory", settings.MaxProjectOverride.Memory, false)
	if err != nil {
		return err
	}
	if settings.MaxProjectOverride.CPU < 0 || settings.MaxProjectOverride.CPU > maxSettableCPU {
		return fmt.Errorf("%w: maxProjectOverride.cpu must be between 0 and %d", ErrInvalidSettings, maxSettableCPU)
	}
	maxDisk, err := checkDisk("maxProjectOverride.disk", settings.MaxProjectOverride.Disk)
	if err != nil {
		return err
	}

	if maxMemory > 0 && maxMemory < defaultMemory {
		return fmt.Errorf("%w: maxProjectOverride.memory is below defaults.memory", ErrInvalidSettings)
	}
	if settings.MaxProjectOverride.CPU > 0 && settings.MaxProjectOverride.CPU < settings.Defaults.CPU {
		return fmt.Errorf("%w: maxProjectOverride.cpu is below defaults.cpu", ErrInvalidSettings)
	}
	if maxDisk > 0 && defaultDisk > 0 && maxDisk < defaultDisk {
		return fmt.Errorf("%w: maxProjectOverride.disk is below defaults.disk", ErrInvalidSettings)
	}
	if settings.MaxRunningContainers < 0 {
		return fmt.Errorf("%w: maxRunningContainers cannot be negative", ErrInvalidSettings)
	}

	if facts.MemoryBytes > 0 {
		if reserve >= facts.MemoryBytes {
			return fmt.Errorf(
				"%w: hostReserve.memory %s leaves nothing of the host's %s for workspaces",
				ErrInvalidSettings, settings.HostReserve.Memory, FormatSize(facts.MemoryBytes),
			)
		}
		if defaultMemory > facts.MemoryBytes-reserve {
			return fmt.Errorf(
				"%w: defaults.memory %s exceeds the %s left after the host reserve",
				ErrInvalidSettings, settings.Defaults.Memory, FormatSize(facts.MemoryBytes-reserve),
			)
		}
	}
	return nil
}

func checkMemory(field, value string, required bool) (uint64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		if required {
			return 0, fmt.Errorf("%w: %s is required", ErrInvalidSettings, field)
		}
		return 0, nil
	}
	bytes, err := ParseSize(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%w: %s is not a byte size like 2GiB", ErrInvalidSettings, field)
	}
	if bytes < minSettableMemory {
		return 0, fmt.Errorf("%w: %s must be at least %s", ErrInvalidSettings, field, FormatSize(minSettableMemory))
	}
	return bytes, nil
}

func checkDisk(field, value string) (uint64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	bytes, err := ParseSize(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%w: %s is not a byte size like 20GiB", ErrInvalidSettings, field)
	}
	if bytes < minSettableDisk || bytes > maxSettableDisk {
		return 0, fmt.Errorf(
			"%w: %s must be between %s and %s",
			ErrInvalidSettings, field, FormatSize(minSettableDisk), FormatSize(maxSettableDisk),
		)
	}
	return bytes, nil
}

// merge overlays the fields an operator actually submitted onto the current
// policy. maxRunningContainers is the exception: zero is the meaningful value
// "unlimited", so the caller sends it explicitly through a pointer.
func merge(current, in Settings) Settings {
	out := current
	if in.Defaults.Memory != "" {
		out.Defaults.Memory = strings.TrimSpace(in.Defaults.Memory)
	}
	if in.Defaults.CPU > 0 {
		out.Defaults.CPU = in.Defaults.CPU
	}
	if in.Defaults.Processes > 0 {
		out.Defaults.Processes = in.Defaults.Processes
	}
	if in.Defaults.Disk != "" {
		out.Defaults.Disk = strings.TrimSpace(in.Defaults.Disk)
	}
	if in.HostReserve.Memory != "" {
		out.HostReserve.Memory = strings.TrimSpace(in.HostReserve.Memory)
	}
	if in.HostReserve.CPU > 0 {
		out.HostReserve.CPU = in.HostReserve.CPU
	}
	if in.MaxProjectOverride.Memory != "" {
		out.MaxProjectOverride.Memory = strings.TrimSpace(in.MaxProjectOverride.Memory)
	}
	if in.MaxProjectOverride.CPU > 0 {
		out.MaxProjectOverride.CPU = in.MaxProjectOverride.CPU
	}
	if in.MaxProjectOverride.Processes > 0 {
		out.MaxProjectOverride.Processes = in.MaxProjectOverride.Processes
	}
	if in.MaxProjectOverride.Disk != "" {
		out.MaxProjectOverride.Disk = strings.TrimSpace(in.MaxProjectOverride.Disk)
	}
	out.MaxRunningContainers = in.MaxRunningContainers
	return out
}

// normalize repairs a hand-edited or partially written document so the rest of
// the service can assume every field is usable.
func normalize(settings Settings) Settings {
	settings.Defaults.Memory = strings.TrimSpace(settings.Defaults.Memory)
	settings.Defaults.Disk = strings.TrimSpace(settings.Defaults.Disk)
	settings.HostReserve.Memory = strings.TrimSpace(settings.HostReserve.Memory)
	settings.MaxProjectOverride.Memory = strings.TrimSpace(settings.MaxProjectOverride.Memory)
	settings.MaxProjectOverride.Disk = strings.TrimSpace(settings.MaxProjectOverride.Disk)

	if _, err := ParseSize(settings.Defaults.Memory); err != nil || settings.Defaults.Memory == "" {
		settings.Defaults.Memory = FormatSize(minDefaultMemory)
	}
	if settings.Defaults.CPU < 1 {
		settings.Defaults.CPU = 1
	}
	if settings.Defaults.Processes < 1 {
		settings.Defaults.Processes = defaultProcesses
	}
	if settings.HostReserve.Memory == "" {
		settings.HostReserve.Memory = DefaultReserveMemory
	}
	if settings.MaxRunningContainers < 0 {
		settings.MaxRunningContainers = 0
	}
	return settings
}
