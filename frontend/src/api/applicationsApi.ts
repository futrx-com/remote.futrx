import { requestJson } from "./apiRequest.ts";
import { API_ROUTES } from "../config/routes.ts";
import type {
  AppCredentials,
  AppImage,
  AppInstance,
  AppInstallRequest,
  AppPackage,
  AppPackageInstall,
  AppUIExtension,
} from "../models/application";

// Global (server-wide) application management. Catalog is shared by the
// project-scoped UI too.
export const applicationsApi = {
  catalog: () =>
    requestJson<AppImage[]>("GET", API_ROUTES.applications.catalog),

  /** Extensions this user should load — installed, running, with their scope. */
  uiExtensions: () =>
    requestJson<AppUIExtension[]>("GET", API_ROUTES.applications.ui),

  /** Uploaded packages, including ones that no longer load. Admin only. */
  packages: () => requestJson<AppPackage[]>("GET", API_ROUTES.applications.packages),

  /**
   * Upload a .zip as a new application, or as a new version of one already
   * uploaded. Sent as multipart so the archive streams rather than being
   * base64-inflated into a JSON body.
   */
  uploadPackage: (file: File) => {
    const form = new FormData();
    form.append("package", file, file.name);
    return requestJson<AppPackage>("POST", API_ROUTES.applications.packages, form);
  },

  /**
   * Delete an uploaded package. `uninstallInstalled` uninstalls every copy
   * first; without it a package that is still installed is refused, so a plain
   * delete can never destroy a database's container as a side effect.
   */
  removePackage: (packageId: string, uninstallInstalled = false) =>
    requestJson<{ ok: boolean; uninstalled: AppPackageInstall[] }>(
      "DELETE",
      API_ROUTES.applications.package(packageId, uninstallInstalled),
    ),

  listGlobal: () =>
    requestJson<AppInstance[]>("GET", API_ROUTES.applications.collection),

  install: (req: AppInstallRequest) =>
    requestJson<AppInstance>("POST", API_ROUTES.applications.collection, req),

  start: (appId: string) =>
    requestJson<AppInstance>("POST", API_ROUTES.applications.action(appId, "start")),

  stop: (appId: string) =>
    requestJson<AppInstance>("POST", API_ROUTES.applications.action(appId, "stop")),

  setPort: (appId: string, port: number) =>
    requestJson<AppInstance>("PUT", API_ROUTES.applications.action(appId, "port"), {
      port,
    }),

  uninstall: (appId: string) =>
    requestJson<{ ok: boolean }>("DELETE", API_ROUTES.applications.item(appId)),

  credentials: (appId: string) =>
    requestJson<AppCredentials>("GET", API_ROUTES.applications.action(appId, "credentials")),
};
