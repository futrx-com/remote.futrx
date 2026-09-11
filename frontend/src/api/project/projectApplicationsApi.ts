import { requestJson } from "../apiRequest";
import { API_ROUTES } from "../../config/routes";
import type {
  AppCredentials,
  AppInstance,
  AppInstallRequest,
} from "../../models/application";

// Project-scoped application management. Spread into projectApi.
export const projectApplicationsApi = {
  listApplications: (id: string) =>
    requestJson<AppInstance[]>("GET", API_ROUTES.projects.applications(id)),

  installApplication: (id: string, req: AppInstallRequest) =>
    requestJson<AppInstance>("POST", API_ROUTES.projects.applications(id), req),

  startApplication: (id: string, appId: string) =>
    requestJson<AppInstance>(
      "POST",
      API_ROUTES.projects.applicationAction(id, appId, "start")
    ),

  stopApplication: (id: string, appId: string) =>
    requestJson<AppInstance>(
      "POST",
      API_ROUTES.projects.applicationAction(id, appId, "stop")
    ),

  setApplicationPort: (id: string, appId: string, port: number) =>
    requestJson<AppInstance>(
      "PUT",
      API_ROUTES.projects.applicationAction(id, appId, "port"),
      { port }
    ),

  uninstallApplication: (id: string, appId: string) =>
    requestJson<{ ok: boolean }>(
      "DELETE",
      API_ROUTES.projects.application(id, appId)
    ),

  applicationCredentials: (id: string, appId: string) =>
    requestJson<AppCredentials>(
      "GET",
      API_ROUTES.projects.applicationAction(id, appId, "credentials")
    ),
};
