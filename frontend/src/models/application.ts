// Installable applications ("Applications" tab): databases and other services
// a user installs with one click, globally or scoped to a project. Mirrors the
// backend service/applications model.

export type AppScope = "global" | "project";

/**
 * What installing an image actually does. `service` runs software in a
 * container (a dedicated one for global scope); `ui` installs nothing anywhere
 * and only turns on the image's browser extension; `backend` installs nothing
 * either, and runs the Go plugin the image ships as a server-side process.
 */
export type AppKind = "service" | "ui" | "backend" | "tool";

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

/** Who the server lets reach an image's plugin. */
export type AppBackendAccess = "registered" | "admin";

/**
 * Go plugin an image ships in its `plugin/` directory. Present only when the
 * image has one; the SPA never sees the source, only that it exists and how it
 * may be called.
 */
export interface AppImageBackend {
  access?: AppBackendAccess;
  timeoutMs?: number;
}

/** One endpoint a running plugin advertises. */
export interface AppBackendRoute {
  method: string;
  path: string;
  description?: string;
}

/** What a running plugin reports about itself. */
export interface AppBackendDescriptor {
  instanceId: string;
  imageId: string;
  descriptor: {
    name: string;
    version?: string;
    apiVersion: number;
    routes?: AppBackendRoute[];
  };
  access: AppBackendAccess;
  timeoutMs: number;
}

/**
 * One running plugin an extension may call. An image installed both globally
 * and in a project runs one process per install, so an extension addresses an
 * instance rather than an image.
 */
export interface AppBackendInstance {
  instanceId: string;
  scope: AppScope;
  projectId?: string;
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
  scopes: AppScope[];
  port: AppPort;
  env?: AppEnvVar[];
  service?: string;
  /** Set when the image ships a `ui/` extension. */
  ui?: AppImageUI;
  /** Set when the image ships a `plugin/` Go backend. */
  backend?: AppImageBackend;
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
  /** Running instances of this image whose plugin the extension may call. */
  backends?: AppBackendInstance[];
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

/**
 * What happened to one installed copy when its app's `version` moved. Re-running
 * an install script touches a container someone is using, so the result is
 * reported back rather than left to a log.
 */
export interface AppUpgradeOutcome {
  instanceId: string;
  name: string;
  scope: AppScope;
  projectId?: string;
  /** Version the copy had recorded; absent if it predates version tracking. */
  fromVersion?: string;
  toVersion: string;
  /** Set when the re-install failed; the copy keeps its old version. */
  error?: string;
}

/** One installed copy of an uploaded package. */
export interface AppPackageInstall {
  instanceId: string;
  name: string;
  scope: AppScope;
  projectId?: string;
  status: AppInstanceStatus;
  /** Set when this copy could not be uninstalled during a removal. */
  error?: string;
}

/**
 * One uploaded application package: a ZIP holding what an `images/<id>/`
 * directory holds. It is stored in the server's state directory rather than in
 * its binary, so updating the server keeps every uploaded application, its
 * installed instances and their settings.
 */
export interface AppPackage {
  id: string;
  name: string;
  version?: string;
  type?: AppKind;
  /**
   * Scopes the packaged app declares. Uploading adds it to a server-wide
   * catalog, which is not the same as making it installable everywhere — a
   * project-only app is listed for every admin and installable only inside a
   * project.
   */
  scopes?: AppScope[];
  /**
   * Copies of this app currently installed, in every scope. Removing a package
   * has to deal with them, so the list is what turns "uninstall it everywhere
   * first" into a decision instead of a dead end.
   */
  installs?: AppPackageInstall[];
  /** Name of the archive it was uploaded from, kept for recognition. */
  filename?: string;
  size: number;
  sha256: string;
  uploadedAt: number;
  uploadedBy?: string;
  /**
   * Installed copies this upload re-provisioned because its `version` differed
   * from theirs. Empty when the version was unchanged, when nothing is
   * installed, or when the app reaches no container.
   */
  upgraded?: AppUpgradeOutcome[];
  /**
   * Why this package is not in the catalog. Set when the files are still on
   * disk but no longer load — an upload made against a different server
   * version, say — so it can be re-uploaded or removed rather than vanishing.
   */
  error?: string;
}
