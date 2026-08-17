import type { ContainerLimits, ContainerState, ResourceInfo } from "./project";

/** One resource envelope as the fleet policy expresses it. */
export interface FleetLimits {
  memory?: string;
  cpu?: number;
  processes?: number;
  disk?: string;
}

/** The slice of the host held back from workspaces. */
export interface FleetReserve {
  memory?: string;
  cpu?: number;
}

/** The operator-editable policy document (DATA_DIR/resources.json). */
export interface FleetSettings {
  defaults: FleetLimits;
  hostReserve: FleetReserve;
  maxProjectOverride: FleetLimits;
  maxRunningContainers: number;
  derived?: boolean;
  updatedAt?: number;
  updatedBy?: string;
}

/** Host capacity and the arithmetic behind the aggregate start guard. */
export interface HostCapacity {
  memoryBytes: number;
  cpus: number;
  diskBytes: number;
  reserveMemoryBytes: number;
  budgetMemoryBytes: number;
  committedMemoryBytes: number;
  runningContainers: number;
}

/**
 * Whether the LXD storage pool can enforce a root-disk quota at all. `dir`
 * pools cannot, so the UI shows the reason instead of a control that silently
 * does nothing.
 */
export interface DiskQuotaSupport {
  supported: boolean;
  pool?: string;
  driver?: string;
  detail?: string;
}

/** GET/PUT /api/admin/resources */
export interface FleetResourcesView {
  settings: FleetSettings;
  host: HostCapacity;
  diskQuota: DiskQuotaSupport;
  runtimeAvailable: boolean;
}

/** The fleet policy one project is measured against. */
export interface ContainerPolicySnapshot {
  defaults: ContainerLimits;
  maxOverride: ContainerLimits;
  maxRunningContainers: number;
  host: HostCapacity;
  diskQuota: DiskQuotaSupport;
}

/** GET/PUT /api/projects/{id}/resources */
export interface ProjectResources {
  policy: ContainerPolicySnapshot;
  overrides?: ContainerLimits;
  effective: ContainerLimits;
  state: ContainerState;
  usage?: ResourceInfo;
  editable: boolean;
  appliedNow: boolean;
  needsRestart: boolean;
}
