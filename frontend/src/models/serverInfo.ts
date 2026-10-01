export interface ServerInfo {
  collectedAt: number;
  host: ServerHostInfo;
  cpu: ServerCPUInfo;
  memory: ServerMemoryInfo;
  storage: ServerStorageInfo;
  network: ServerNetworkInfo;
  process: ServerProcessInfo;
}

export interface ServerHostInfo {
  hostname: string;
  os: string;
  platform?: string;
  architecture: string;
  kernel?: string;
  uptimeSec?: number;
  bootedAt?: number;
  serviceUptimeSec: number;
  appVersion: string;
  goVersion: string;
  dataPath: string;
  workspacePath: string;
}

export interface ServerCPUInfo {
  logicalCores: number;
  model?: string;
  usagePercent?: number;
  loadAverage1?: number;
  loadAverage5?: number;
  loadAverage15?: number;
}

export interface ServerMemoryInfo {
  totalBytes: number;
  usedBytes: number;
  availableBytes: number;
  freeBytes: number;
  cachedBytes: number;
  buffersBytes: number;
  usagePercent: number;
  swapTotalBytes: number;
  swapUsedBytes: number;
  swapFreeBytes: number;
}

export interface ServerStorageInfo {
  totalBytes: number;
  usedBytes: number;
  availableBytes: number;
  usagePercent: number;
  mounts: ServerStorageMount[];
}

export interface ServerStorageMount {
 inodePercent?: number;
 warning?: boolean;
  device?: string;
  mountPath: string;
  filesystem?: string;
  totalBytes: number;
  usedBytes: number;
  availableBytes: number;
  usagePercent: number;
}

export interface ServerNetworkInfo {
  receivedBytes: number;
  sentBytes: number;
  interfaces: ServerNetworkInterface[];
}

export interface ServerNetworkInterface {
  name: string;
  mtu?: number;
  hardwareAddress?: string;
  addresses?: string[];
  receivedBytes: number;
  sentBytes: number;
  loopback: boolean;
  up: boolean;
}

export interface ServerProcessInfo {
  pid: number;
  goroutines: number;
  openFileHandles?: number;
  allocatedBytes: number;
  heapInUseBytes: number;
  systemMemoryBytes: number;
}
