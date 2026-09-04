// Installable applications ("Applications" tab): databases and other services
// a user installs with one click, globally or scoped to a project. Mirrors the
// backend service/applications model.

export type AppScope = "global" | "project";

/**
 * What installing an image actually does. `service` runs software in a
 * container (a dedicated one for global scope); `ui` installs nothing anywhere
 * and only turns on the image's browser extension.
 */
export type AppKind = "service" | "ui";

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
 * Browser-side extension an image ships in its `ui/` directory. Present only
 * when the image has one; paths are relative to `ui/` and already validated by
 * the backend, so the SPA can load them without existence checks.
 */
export interface AppImageUI {
  /** ES module whose default export is called with the extension API. */
  entry?: string;
  /** Stylesheets injected into the document, in order. */
  styles?: string[];
  /** HTML fragments the entry module loads by name. */
  views?: Record<string, string>;
}

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
  type: AppKind;
  scopes: AppScope[];
  port: AppPort;
  env?: AppEnvVar[];
  service?: string;
  /** Set when the image ships a `ui/` extension. */
  ui?: AppImageUI;
}

/**
 * One extension the signed-in user should load, with where it was installed.
 * A globally installed image applies everywhere; a project-installed one only
 * inside those projects.
 */
export interface AppUIExtension {
  image: AppImage;
  global: boolean;
  projectIds?: string[];
}

/** API-safe view of one installed instance (secret env values redacted). */
export interface AppInstance {
  id: string;
  imageId: string;
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
