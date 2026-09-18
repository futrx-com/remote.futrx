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
 * Browser-side extension an application ships in its `ui/` directory. Present only
 * when the application has one; paths are relative to `ui/` and already validated by
 * the backend, so the SPA can load them without existence checks.
 */
export interface AppApplicationUI {
  /** ES module whose default export is called with the extension API. */
  entry?: string;
  /** Stylesheets injected into the document, in order. */
  styles?: string[];
  /** HTML fragments the entry module loads by name. */
  views?: Record<string, string>;
}

/** Who the server lets reach an application's plugin. */
export type AppBackendAccess = "registered" | "admin";

/**
 * Go plugin an application ships in its `backend/` directory. Present only when the
 * application has one; the SPA never sees the source, only that it exists and how it
 * may be called.
 */
export interface AppApplicationBackend {
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
  applicationId: string;
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
 * One running plugin an extension may call. An application installed both globally
 * and in a project runs one process per install, so an extension addresses an
 * instance rather than an application.
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
   * Capabilities inferred by the server from infra/, backend/, and ui/.
   */
  needsContainer?: boolean;
  needsPort?: boolean;
  scopes: AppScope[];
  port: AppPort;
  env?: AppEnvVar[];
  service?: string;
  /** Set when the application ships a `ui/` extension. */
  ui?: AppApplicationUI;
  /** Set when the application ships a `backend/` Go backend. */
  backend?: AppApplicationBackend;
}

/**
 * One extension the signed-in user should load, with where it was installed.
 * A globally installed application applies everywhere; a project-installed one only
 * inside those projects.
 */
export interface AppUIExtension {
  application: AppApplication;
  global: boolean;
  projectIds?: string[];
  /** Running instances of this application whose plugin the extension may call. */
  backends?: AppBackendInstance[];
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
