// Installable applications ("Applications" tab): databases and other services
// a user installs with one click, globally or scoped to a project. Mirrors the
// backend service/applications model.

export type AppScope = "global" | "project";

export type AppInstanceStatus =
  | "installing"
  | "running"
  | "stopped"
  | "error";

export interface AppEnvVar {
  key: string;
  label?: string;
  required?: boolean;
  secret?: boolean;
  default?: string;
  generate?: string;
}

export interface AppPort {
  internal: number;
  defaultExternal: number;
  protocol?: string;
  bindAddress?: string;
}

/**
 * Where a catalog entry came from. `builtin` entries ship inside the server;
 * `uploaded` ones came from a package an administrator uploaded and can be
 * removed again.
 */
export type AppApplicationSource = "builtin" | "uploaded";

/** One catalog entry loaded from applications/<id>/application.json. */
export interface AppApplication {
  id: string;
  name: string;
  description?: string;
  category?: string;
  version?: string;
  /**
   * Built-in icon key (`"database"`, `"cache"`, …) or a path to an image the
   * catalog entry ships in its own `ui/` (`"ui/assets/logo.svg"`).
   */
  icon?: string;
  /** Decided by the server, never by the package: see {@link AppApplicationSource}. */
  source?: AppApplicationSource;
  /**
   * Capabilities inferred by the server from the package layout.
   */
  needsContainer?: boolean;
  needsPort?: boolean;
  scopes: AppScope[];
  port: AppPort;
  env?: AppEnvVar[];
  service?: string;
}

/** API-safe view of one installed instance (secret env values redacted). */
export interface AppInstance {
  id: string;
  applicationId: string;
  /** The application.json version this copy was last installed from. */
  applicationVersion?: string;
  name: string;
  scope: AppScope;
  projectId?: string;
  containerName: string;
  deviceName: string;
  internalPort: number;
  externalPort: number;
  bindAddress: string;
  protocol?: string;
  status: AppInstanceStatus;
  error?: string;
  createdAt: number;
  updatedAt: number;
  /** Non-secret env values, keyed by var name. */
  envPublic?: Record<string, string>;
}

/** Full connection details for an installed app, including secret env. */
export interface AppCredentials {
  containerName: string;
  lxdHost: string;
  internalPort: number;
  externalPort: number;
  bindAddress: string;
  username?: string;
  password?: string;
  database?: string;
  env?: Record<string, string>;
}

/** Payload for installing an app. */
export interface AppInstallRequest {
  applicationId: string;
  name?: string;
  env?: Record<string, string>;
  externalPort?: number;
  bindAddress?: string;
}
