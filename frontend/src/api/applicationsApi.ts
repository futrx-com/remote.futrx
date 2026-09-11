import { requestJson } from "./apiRequest.ts";
import { API_ROUTES } from "../config/routes.ts";
import type {
  AppCredentials,
  AppImage,
  AppInstance,
  AppInstallRequest,
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
