import { requestJson } from "./apiRequest";
import { API_ROUTES } from "../config/routes";
import type { SelfUpdateReleaseNotes, SelfUpdateStatus } from "../models/selfUpdate";

export const selfUpdateApi = {
  status: () => requestJson<SelfUpdateStatus>("GET", API_ROUTES.selfUpdate.status),
  releaseNotes: (tag: string) => requestJson<SelfUpdateReleaseNotes>("GET", API_ROUTES.selfUpdate.releaseNotes(tag)),
  check: () => requestJson<SelfUpdateStatus>("POST", API_ROUTES.selfUpdate.check),
  apply: (tag?: string) =>
    requestJson<SelfUpdateStatus>(
      "POST",
      API_ROUTES.selfUpdate.apply,
      tag ? { tag } : {}
    ),
};
