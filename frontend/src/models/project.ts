export type ProjectStatus =
  | ""
  | "provisioning"
  | "running"
  | "stopped"
  | "error"
  | "missing";

export type ContainerState =
  | "RUNNING"
  | "STOPPED"
  | "FROZEN"
  | "MISSING"
  | "UNKNOWN";

export interface ProjectMeta {
  id: string;
  name: string;
  slug: string;
  cwd: string;
  containerName: string;
  status: ProjectStatus;
  order?: number;
  errorMsg?: string;
  resourceLimits?: ContainerLimits;
  createdAt: number;
  updatedAt: number;
}

export interface WorkspaceInfo {
  hostSource: string;
  containerPath: string;
}

export interface ResourceInfo {
  processes?: number;
  diskUsageBytes?: number;
  memoryCurrentBytes?: number;
  memoryPeakBytes?: number;
  memoryTotalBytes?: number;
  swapCurrentBytes?: number;
  cpuUsageSeconds?: number;
}

export interface NetworkInterface {
  name: string;
  state?: string;
  type?: string;
  hostName?: string;
  macAddress?: string;
  mtu?: number;
  addresses?: string[];
  bytesReceived?: number;
  bytesSent?: number;
}

export interface OSInfo {
  prettyName?: string;
  kernel?: string;
  uptimeSec?: number;
  cpuCount?: number;
  hostname?: string;
}

export interface DiskUsage {
  mountPath: string;
  filesystem?: string;
  totalBytes?: number;
  usedBytes?: number;
  availBytes?: number;
}

export interface ContainerLimits {
  cpu?: string;
  memory?: string;
  disk?: string;
}

export interface ClaudeContainerStatus {
  installed: boolean;
  version?: string;
  claudeMdInstalled: boolean;
  claudeMdInSync: boolean;
}

export interface CodexContainerStatus {
  installed: boolean;
  version?: string;
}

export interface AgentContainerStatus {
  id: string;
  label: string;
  installed: boolean;
  version?: string;
  instructionsPath?: string;
  instructionsInstalled?: boolean;
  instructionsInSync?: boolean;
}

export interface AuthBundleFileStatus {
  hostPath: string;
  containerPath: string;
  hostExists: boolean;
  hostMtime?: number;
  containerExists: boolean;
  containerMtime?: number;
  hostNewer: boolean;
  containerNewer: boolean;
}

export interface AuthBundleStatus {
  name: string;
  files: AuthBundleFileStatus[];
}

export interface ProjectContainerInfo {
  name: string;
  state: ContainerState;
  bootAutostart: boolean;
  image?: string;
  type?: string;
  architecture?: string;
  pid?: number;
  createdAt?: string;
  lastUsedAt?: string;
  workspace?: WorkspaceInfo;
  resources?: ResourceInfo;
  network?: NetworkInterface[];
  os?: OSInfo;
  disks?: DiskUsage[];
  limits?: ContainerLimits;
  limitOverrides?: ContainerLimits;
  claude: ClaudeContainerStatus;
  codex: CodexContainerStatus;
  agents?: AgentContainerStatus[];
  authBundles: AuthBundleStatus[];
}

export interface ProjectSecret {
  key: string;
  value: string;
  updatedAt: number;
}

export interface ContainerApp {
  port: number;
  address?: string;
  process?: string;
  pid?: number;
}

export type AgentBrowserStatus = "idle" | "starting" | "ready" | "core-ready" | "error" | "stopped";
export type AgentBrowserServerStatus = Exclude<AgentBrowserStatus, "idle">;

export interface AgentBrowserInfo {
  status: AgentBrowserServerStatus;
  url?: string;
  slug?: string;
  port?: number;
  error?: string;
  core?: string;
  view?: string;
  viewerCount?: number;
  uptimeSec?: number;
  lastActivity?: number;
}
