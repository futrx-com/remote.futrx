import { requestJson } from "./apiRequest";
import { API_ROUTES } from "../config/routes";
import type { SelfUpdateStatus } from "../models/selfUpdate";

export const selfUpdateApi = {
  status: () => requestJson<SelfUpdateStatus>("GET", API_ROUTES.selfUpdate.status),
  check: () => requestJson<SelfUpdateStatus>("POST", API_ROUTES.selfUpdate.check),
  apply: (tag?: string) =>
    requestJson<SelfUpdateStatus>(
      "POST",
      API_ROUTES.selfUpdate.apply,
      tag ? { tag } : {}
    ),
  retry: () => requestJson<SelfUpdateStatus>("POST", API_ROUTES.selfUpdate.retry),
};
