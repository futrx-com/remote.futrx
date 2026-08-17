import { requestJson } from "../apiRequest";
import type { ProjectShare } from "../../models/project";
import { API_ROUTES } from "../../config/routes";

export const projectSharesApi = {
  listShares: (id: string) =>
    requestJson<ProjectShare[]>("GET", API_ROUTES.projects.shares(id)),

  createShare: (
    id: string,
    body: { port: number; ttlHours: number; label?: string },
  ) => requestJson<ProjectShare>("POST", API_ROUTES.projects.shares(id), body),

  revokeShare: (id: string, shareId: string) =>
    requestJson<{ ok: boolean }>(
      "DELETE",
      API_ROUTES.projects.share(id, shareId),
    ),
};
