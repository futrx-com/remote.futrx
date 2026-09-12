import type { ContainerLimits, ResourceInfo } from "../../models/project";
import type { FleetLimits, HostCapacity } from "../../models/resources";

/** LXD byte-size grammar, mirrored from the backend's own validation. */
const SIZE_PATTERN = /^[1-9][0-9]*(?:\.[0-9]+)?(MiB|GiB|TiB)$/;

const UNIT_BYTES: Record<string, number> = {
  MiB: 1024 ** 2,
  GiB: 1024 ** 3,
  TiB: 1024 ** 4,
};

const MAX_CPU_CORES = 256;

/**
 * Parses an LXD byte-size literal. Returns undefined for anything the backend
 * would also refuse, so the form and the API agree on what is valid.
 */
export function parseSize(value?: string): number | undefined {
  const trimmed = (value ?? "").trim();
  if (!trimmed) return undefined;
  const match = SIZE_PATTERN.exec(trimmed);
  if (!match) return undefined;
  const amount = Number.parseFloat(trimmed.slice(0, trimmed.length - match[1].length));
  if (!Number.isFinite(amount)) return undefined;
  return amount * UNIT_BYTES[match[1]];
}

/** Renders bytes as the largest binary unit that divides them exactly. */
export function formatSize(bytes?: number): string {
  if (bytes == null || !Number.isFinite(bytes) || bytes <= 0) return "—";
  for (const unit of ["TiB", "GiB", "MiB"] as const) {
    const size = UNIT_BYTES[unit];
    if (bytes >= size && bytes % size === 0) return `${bytes / size}${unit}`;
  }
  if (bytes >= UNIT_BYTES.MiB) return `${Math.round(bytes / UNIT_BYTES.MiB)}MiB`;
  return `${bytes}B`;
}

/**
 * Validates one per-project override against the fleet ceiling. Returning the
 * first problem (rather than a list) keeps the form's single error line honest
 * about what to fix next.
 */
export function validateOverride(
  limits: ContainerLimits,
  ceiling?: ContainerLimits
): string | undefined {
  const cpu = (limits.cpu ?? "").trim();
  if (cpu) {
    const cores = Number(cpu);
    if (!Number.isInteger(cores) || cores < 1 || cores > MAX_CPU_CORES) {
      return `CPU must be a whole number from 1 to ${MAX_CPU_CORES}.`;
    }
    const maxCores = Number((ceiling?.cpu ?? "").trim());
    if (Number.isFinite(maxCores) && maxCores > 0 && cores > maxCores) {
      return `CPU cannot exceed the fleet maximum of ${maxCores}.`;
    }
  }

  const memory = (limits.memory ?? "").trim();
  if (memory) {
    const bytes = parseSize(memory);
    if (bytes == null) return "Memory must use MiB, GiB, or TiB, for example 3GiB.";
    const ceilingBytes = parseSize(ceiling?.memory);
    if (ceilingBytes != null && bytes > ceilingBytes) {
      return `Memory cannot exceed the fleet maximum of ${ceiling?.memory}.`;
    }
  }

  const disk = (limits.disk ?? "").trim();
  if (disk) {
    const bytes = parseSize(disk);
    if (bytes == null) return "Disk must use MiB, GiB, or TiB, for example 40GiB.";
    const ceilingBytes = parseSize(ceiling?.disk);
    if (ceilingBytes != null && bytes > ceilingBytes) {
      return `Disk cannot exceed the fleet maximum of ${ceiling?.disk}.`;
    }
  }

  return undefined;
}

/** Validates the fleet policy form before it reaches the server. */
export function validateFleetSettings(
  defaults: FleetLimits,
  reserveMemory: string,
  maxOverride: FleetLimits,
  maxRunningContainers: number,
  host?: HostCapacity
): string | undefined {
  const defaultBytes = parseSize(defaults.memory);
  if (defaultBytes == null) return "Default memory must use MiB, GiB, or TiB, for example 2GiB.";
  if (defaultBytes < 256 * UNIT_BYTES.MiB) return "Default memory must be at least 256MiB.";

  if (!Number.isInteger(defaults.cpu ?? 0) || (defaults.cpu ?? 0) < 1) {
    return "Default CPU must be a whole number of at least 1.";
  }
  if (!Number.isInteger(defaults.processes ?? 0) || (defaults.processes ?? 0) < 1) {
    return "Default process limit must be a whole number of at least 1.";
  }
  if (defaults.disk && parseSize(defaults.disk) == null) {
    return "Default disk quota must use MiB, GiB, or TiB, for example 20GiB.";
  }

  const reserveBytes = parseSize(reserveMemory);
  if (reserveBytes == null) return "Host reserve must use MiB, GiB, or TiB, for example 768MiB.";

  const maxMemoryBytes = parseSize(maxOverride.memory);
  if (maxOverride.memory && maxMemoryBytes == null) {
    return "Maximum project memory must use MiB, GiB, or TiB.";
  }
  if (maxMemoryBytes != null && maxMemoryBytes < defaultBytes) {
    return "Maximum project memory cannot be below the default.";
  }
  if ((maxOverride.cpu ?? 0) > 0 && (maxOverride.cpu ?? 0) < (defaults.cpu ?? 0)) {
    return "Maximum project CPU cannot be below the default.";
  }
  const maxDiskBytes = parseSize(maxOverride.disk);
  const defaultDiskBytes = parseSize(defaults.disk);
  if (maxDiskBytes != null && defaultDiskBytes != null && maxDiskBytes < defaultDiskBytes) {
    return "Maximum project disk cannot be below the default.";
  }

  if (!Number.isInteger(maxRunningContainers) || maxRunningContainers < 0) {
    return "Maximum running containers must be 0 (unlimited) or a positive whole number.";
  }

  if (host?.memoryBytes) {
    if (reserveBytes >= host.memoryBytes) {
      return `Host reserve leaves nothing of the host's ${formatSize(host.memoryBytes)} for workspaces.`;
    }
    if (defaultBytes > host.memoryBytes - reserveBytes) {
      return `Default memory exceeds the ${formatSize(host.memoryBytes - reserveBytes)} left after the host reserve.`;
    }
  }

  return undefined;
}

/** One usage bar: how much of a limit is currently consumed. */
export interface UsageMeter {
  label: string;
  usedBytes?: number;
  limitBytes?: number;
  percent?: number;
  detail: string;
}

/**
 * Builds the memory and disk meters shown beside the form. A meter without a
 * limit still reports usage; a meter without usage reports the limit alone, so
 * a stopped container degrades to plain text rather than a misleading 0% bar.
 */
export function usageMeters(
  usage: ResourceInfo | undefined,
  effective: ContainerLimits
): UsageMeter[] {
  const memoryLimit = parseSize(effective.memory) ?? usage?.memoryTotalBytes;
  const diskLimit = parseSize(effective.disk);
  return [
    meter("Memory", usage?.memoryCurrentBytes, memoryLimit),
    meter("Disk", usage?.diskUsageBytes, diskLimit),
  ];
}

function meter(label: string, usedBytes?: number, limitBytes?: number): UsageMeter {
  const usable = usedBytes != null && usedBytes > 0;
  const bounded = limitBytes != null && limitBytes > 0;
  return {
    label,
    usedBytes: usable ? usedBytes : undefined,
    limitBytes: bounded ? limitBytes : undefined,
    percent: usable && bounded ? clampPercent((usedBytes / limitBytes) * 100) : undefined,
    detail: describeMeter(usable ? usedBytes : undefined, bounded ? limitBytes : undefined),
  };
}

function describeMeter(usedBytes?: number, limitBytes?: number): string {
  if (usedBytes != null && limitBytes != null) {
    return `${formatSize(usedBytes)} of ${formatSize(limitBytes)}`;
  }
  if (limitBytes != null) return `limit ${formatSize(limitBytes)}`;
  if (usedBytes != null) return `${formatSize(usedBytes)} used, no quota`;
  return "no quota";
}

function clampPercent(value: number): number {
  if (!Number.isFinite(value) || value < 0) return 0;
  return Math.min(100, Math.round(value * 10) / 10);
}

/** How much of the host's workspace budget running containers already claim. */
export function hostCommitmentPercent(host?: HostCapacity): number | undefined {
  if (!host?.budgetMemoryBytes) return undefined;
  return clampPercent((host.committedMemoryBytes / host.budgetMemoryBytes) * 100);
}
