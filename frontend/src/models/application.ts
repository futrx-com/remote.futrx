// Installable applications ("Applications" tab): databases and other services
// a user installs with one click, globally or scoped to a project. Mirrors the
// backend service/applications model.

export type AppScope = "global" | "project";

/**
 * What installing an image actually does. `service` runs software on a port in
 * a container (a dedicated one for global scope); `tool` provisions software
 * into the project's container and exposes nothing.
 */
export type AppKind = "service" | "tool";

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
export type AppImageSource = "builtin" | "uploaded";

/** One catalog entry loaded from images/<id>/image.json. */
export interface AppImage {
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
  /** Decided by the server, never by the package: see {@link AppImageSource}. */
  source?: AppImageSource;
  type: AppKind;
  /**
   * What this entry's kind means for the UI, decided by the server. `service`
   * needs both, `tool` a container but no port — but that mapping belongs to
   * the server that owns the kinds, so the SPA reads these instead of
   * restating it.
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
  imageId: string;
  /** The image.json version this copy was last installed from. */
  imageVersion?: string;
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
  imageId: string;
  name?: string;
  env?: Record<string, string>;
  externalPort?: number;
  bindAddress?: string;
}
